package main

import (
	"time"

	log "github.com/sirupsen/logrus"
	"github.com/spf13/viper"
)

var defaultHostCertLifetime string = "720h"

type service struct {
	name            string
	certPaths       []string
	minCertLifetime time.Duration
}

func newServiceFromConfig(serviceKey, overrideName string) *service {
	var err error
	s := service{}
	if overrideName != "" {
		s.name = overrideName
	} else {
		s.name = serviceKey
	}

	s.certPaths = viper.GetStringSlice(serviceKey + ".certPaths")

	if viper.GetString(serviceKey+".minCertLifetime") == "" {
		// Use global lifetime
		s.minCertLifetime, err = time.ParseDuration(viper.GetString("global.minCertLifetime"))
		if err != nil {
			log.WithField("service", overrideName).Errorf("Could not parse duration for certificate lifetime (global).  Using default of %s", defaultHostCertLifetime)
			s.minCertLifetime, _ = time.ParseDuration(defaultHostCertLifetime)
		}
	} else {
		// Use service-specific lifetime
		s.minCertLifetime, err = time.ParseDuration(viper.GetString(serviceKey + ".minCertLifetime"))
		if err != nil {
			log.WithField("service", overrideName).Error(err)
			log.WithField("service", overrideName).Errorf("Could not parse duration for certificate lifetime.  Using default of %s", defaultHostCertLifetime)
			s.minCertLifetime, _ = time.ParseDuration(defaultHostCertLifetime)
		}
	}
	return &s
}
