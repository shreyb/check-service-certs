package main

import (
	"context"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"io"
	"math"
	"os"
	"strings"
	"sync"
	"text/template"
	"time"

	"github.com/rifflock/lfshook"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
)

var (
	buildTimestamp string
	version        string
	services       = make([]*service, 0)
)

// Initial setup.  Read flags, find config file
func init() {
	const configFile string = "checkServiceCerts"

	// Defaults
	viper.SetDefault("notifications.admin_email", "fife-group@fnal.gov")

	// Flags
	pflag.StringP("configfile", "c", "", "Specify alternate config file")
	pflag.StringP("service", "s", "", "Specify service to run check on")
	pflag.BoolP("test", "t", false, "Test mode.  Check certs, but do not send emails")
	pflag.Bool("version", false, "Version of check-service-certs")

	pflag.Parse()
	viper.BindPFlags(pflag.CommandLine)

	if viper.GetBool("version") {
		fmt.Printf("Check-service-certs library version %s, build %s\n", version, buildTimestamp)
		os.Exit(0)
	}

	// Get config file
	// Check for override
	if config := viper.GetString("configfile"); config != "" {
		viper.SetConfigFile(config)
	} else {
		viper.SetConfigName(configFile)
	}

	viper.AddConfigPath("/etc/check-service-certs/")
	viper.AddConfigPath("$HOME/.check-service-certs/")
	viper.AddConfigPath(".")
	err := viper.ReadInConfig()
	if err != nil {
		log.Panicf("Fatal error reading in config file: %v", err)
	}
}

// Set up logs
func init() {
	log.SetLevel(log.DebugLevel)
	logConfigLookup := "global.logfile"
	// Info log file
	log.AddHook(lfshook.NewHook(lfshook.PathMap{
		log.InfoLevel:  viper.GetString(logConfigLookup),
		log.WarnLevel:  viper.GetString(logConfigLookup),
		log.ErrorLevel: viper.GetString(logConfigLookup),
		log.FatalLevel: viper.GetString(logConfigLookup),
		log.PanicLevel: viper.GetString(logConfigLookup),
	}, &log.TextFormatter{FullTimestamp: true}))

	log.Infof("Using config file %s", viper.ConfigFileUsed())

	if viper.GetBool("test") {
		log.Info("Running in test mode")
	}
}

// Get list of services to run on
func init() {
	if viper.GetString("service") != "" {
		// We've been given a service.  Just use that and move on
		s := newServiceFromConfig("namedServices."+viper.GetString("service"), viper.GetString("service"))
		services = append(services, s)
		return
	}
	// Go through config file, grab all services
	// Unnamed services
	s := newServiceFromConfig("global", "Unnamed Service")
	services = append(services, s)

	// Named services
	namedServiceMap := viper.GetStringMap("namedServices")
	serviceSetupChannel := make(chan *service)

	// Listener to add service to services slice
	listenerDone := make(chan struct{})
	go func() {
		for s := range serviceSetupChannel {
			services = append(services, s)
		}
		close(listenerDone)
	}()

	var serviceSetupWg sync.WaitGroup
	for namedService := range namedServiceMap {
		serviceSetupWg.Add(1)
		go func(namedService string) {
			defer serviceSetupWg.Done()
			s := newServiceFromConfig("namedServices."+namedService, namedService)
			serviceSetupChannel <- s
		}(namedService)
	}
	serviceSetupWg.Wait()
	close(serviceSetupChannel)
	<-listenerDone // Don't move on until we have all our services registered in services slice
}

// Order of operations
// 0.  Setup (global context, set up admin notification emails)
// 1.  Ingest service cert
// 2.  Check if time left is less than configured time
// 3.  If so, send notification.  If not, log and exit
// 4.  Send necessary notifications
func main() {
	//0.
	// Global Context
	var globalTimeout time.Duration
	globalTimeoutDefault, _ := time.ParseDuration("2m")
	var err error

	globalTimeout, err = time.ParseDuration(viper.GetString("global.timeout"))
	if err != nil {
		log.Errorf("Could not parse global timeout.  Using default of %v", globalTimeoutDefault)
		globalTimeout = globalTimeoutDefault
	}
	ctx, cancel := context.WithTimeout(context.Background(), globalTimeout)
	defer cancel()

	// Prep our notifications in advance
	type messageToSend struct {
		Message
		text        string
		serviceName string
	}

	var configNotificationsPrefix string
	if viper.GetBool("test") {
		configNotificationsPrefix = "notifications_test."
	} else {
		configNotificationsPrefix = "notifications."
	}

	// Listener that sends messages as they're ready to go
	notificationsChan := make(chan messageToSend)
	notificationsSent := make(chan struct{})
	go func() {
		// Step 4 is really happening here
		if viper.GetBool("test") {
			log.Info("Running in test mode.  Will not send messages")
		}
		var notificationsWg sync.WaitGroup
		for m := range notificationsChan {
			notificationsWg.Add(1)
			go func(m messageToSend) {
				defer notificationsWg.Done()
				if err := m.Message.sendMessage(ctx, m.text); err != nil {
					log.WithFields(log.Fields{
						"notificationType": fmt.Sprintf("%T", m.Message),
						"service":          m.serviceName,
					}).Errorf("Error sending notification: %s", err)
					return
				}
				log.WithFields(log.Fields{
					"notificationType": fmt.Sprintf("%T", m.Message),
					"service":          m.serviceName,
				}).Info("Notification Sent")
			}(m)
		}
		notificationsWg.Wait()
		close(notificationsSent)
	}()

	// Now go through our services and check the certs for each
	var serviceWg sync.WaitGroup
	for _, s := range services {
		serviceWg.Add(1)
		go func(s *service) {
			defer serviceWg.Done()
			now := time.Now()
			nowString := now.Format(time.RFC822)

			// 1.  Ingest service certificate
			// Borrowed this heavily from own code at
			// https://cdcvs.fnal.gov/redmine/projects/discompsupp/repository/ken_proxy_push/revisions/master/entry/proxy/serviceCert.go

			var certWg sync.WaitGroup
			for _, certPath := range s.certPaths {
				certWg.Add(1)
				go func(certPath string) {
					defer certWg.Done()

					certFile, err := os.Open(certPath)
					if err != nil {
						if os.IsNotExist(err) {
							errText := "certPath does not exist"
							log.WithFields(
								log.Fields{
									"certPath": certPath,
									"service":  s.name,
								}).Error(errText)
							return
						}
						errText := "Could not open service certificate file"
						log.WithFields(
							log.Fields{
								"certPath": certPath,
								"service":  s.name,
							}).Error(errText)
						return
					}

					defer certFile.Close()

					certContent, err := io.ReadAll(certFile)
					if err != nil {
						err := fmt.Sprintf("Could not read cert file: %s", err.Error())
						log.WithFields(
							log.Fields{
								"certPath": certPath,
								"service":  s.name,
							}).Error(err)
						return
					}

					certDER, _ := pem.Decode(certContent)
					if certDER == nil {
						log.WithFields(
							log.Fields{
								"certPath": certPath,
								"service":  s.name,
							}).Error("Could not decode PEM block containing cert data")
						return
					}

					cert, err := x509.ParseCertificate(certDER.Bytes)
					if err != nil {
						err := fmt.Sprintf("Could not parse certificate from DER data: %s", err.Error())
						log.WithFields(
							log.Fields{
								"certPath": certPath,
								"service":  s.name,
							}).Error(err)
						return
					}

					log.WithFields(log.Fields{
						"certPath": certPath,
						"service":  s.name,
					}).Debug("Read in and decoded cert file, getting expiration")

					expiration := cert.NotAfter
					log.WithFields(log.Fields{
						"certPath":   certPath,
						"expiration": expiration,
						"service":    s.name,
					}).Debug("Successfully ingested service certificate")

					// 2.  Check if time left is less than configured time
					if now.Add(s.minCertLifetime).After(expiration) {
						log.WithFields(log.Fields{
							"now":                    now,
							"expiration":             expiration,
							"serviceCertificatePath": certPath,
							"service":                s.name,
						}).Warnf("Service certificate will expire within %s", s.minCertLifetime)
					} else {
						log.WithFields(log.Fields{
							"now":                    now,
							"expiration":             expiration,
							"serviceCertificatePath": certPath,
							"service":                s.name,
						}).Infof("Service certificate will not expire within %s", s.minCertLifetime)
						return
					}

					if viper.GetBool("test") {
						return
					}

					// 3.  If so, send notification.
					// Prepare notifications
					emailToSend := NewEmail(
						viper.GetString("email.from"),
						viper.GetStringSlice(configNotificationsPrefix+"admin_email"),
						fmt.Sprintf("Service Certificates Check (%s), %s", s.name, nowString),
						viper.GetString("email.smtphost"),
						viper.GetInt("email.smtpport"),
						"",
					)
					slackMessageToSend := NewSlackMessage(
						viper.GetString(configNotificationsPrefix + "slack_alerts_url"),
					)

					// Get the text for the notifications
					templateData, err := os.ReadFile(viper.GetString("global.template"))
					if err != nil {
						log.WithFields(log.Fields{
							"templatePath": viper.GetString("templates.expiringCertificate"),
							"service":      s.name,
						}).Errorf("Could not read expiring certificate template file: %s", err)
						return
					}

					var b strings.Builder
					numDays := int(math.Floor(expiration.Sub(now).Hours() / 24.0))
					tmpl := template.Must(template.New("expiringCert").Parse(string(templateData)))
					messageArgs := struct {
						ServiceName string
						CertPath    string
						NumDays     int
					}{
						ServiceName: s.name,
						CertPath:    certPath,
						NumDays:     numDays,
					}

					if err = tmpl.Execute(&b, messageArgs); err != nil {
						log.WithField("service", s.name).Errorf("Failed to execute expiring certificate template: %s", err)
						return
					}

					// Put our notifications into the notificationsChan to send out
					text := b.String()
					adminNotifications := []messageToSend{
						{emailToSend, text, s.name},
						{slackMessageToSend, text, s.name},
					}
					for _, n := range adminNotifications {
						notificationsChan <- n
					}
				}(certPath)
			}
			certWg.Wait()
		}(s)
	}
	serviceWg.Wait()
	close(notificationsChan)
	<-notificationsSent
	log.Info("Finished run")
}
