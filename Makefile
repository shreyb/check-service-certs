NAME = check-service-certs
VERSION = v0.1.1
ROOTDIR = $(shell pwd)
BUILD = $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
rpmVersion := $(subst v,,$(VERSION))
buildTarName = $(NAME)-$(rpmVersion)
buildTarPath = $(ROOTDIR)/$(buildTarName).tar.gz
SOURCEDIR = $(ROOTDIR)/$(buildTarName)
executable = check-service-certs
specfile := $(ROOTDIR)/packaging/$(NAME).spec

all: build tarball spec rpm
.PHONY: all clean build tarball spec rpm

rpm: rpmSourcesDir := $$HOME/rpmbuild/SOURCES
rpm: rpmSpecsDir := $$HOME/rpmbuild/SPECS
rpm: rpmDir := $$HOME/rpmbuild/RPMS/x86_64/
rpm: spec tarball
	cp $(specfile) $(rpmSpecsDir)/
	cp $(buildTarPath) $(rpmSourcesDir)/
	cd $(rpmSpecsDir); \
	rpmbuild -ba ${NAME}.spec
	find $$HOME/rpmbuild/RPMS -type f -name "$(NAME)-$(rpmVersion)*.rpm" -cmin 1 -exec cp {} $(ROOTDIR)/ \;
	echo "Created RPM and copied it to current working directory"


spec:
	sed -Ei 's/Version\:[ ]*.+/Version:        $(rpmVersion)/' $(specfile)
	echo "Set version in spec file to $(rpmVersion)"


tarball: build
	mkdir -p $(SOURCEDIR)
	cp $(executable) $(SOURCEDIR)  # Executables
	cp $(ROOTDIR)/checkServiceCerts.yml $(ROOTDIR)/packaging/check-service-certs.logrotate $(ROOTDIR)/packaging/check-service-certs.cron $(SOURCEDIR)  # Config files
	cp $(ROOTDIR)/expiringCertificate.txt $(SOURCEDIR)/expiringCertificate.txt
	tar -czf $(buildTarPath) -C $(ROOTDIR) $(buildTarName)
	echo "Built deployment tarball"


build:
	echo "Building $(executable)"; \
	go build -ldflags="-X main.buildTimestamp=$(BUILD) -X main.version=$(VERSION)";  \
	echo "Built $(executable)"; \


clean:
	(test -e $(buildTarPath)) && (rm $(buildTarPath))
	(test -e $(SOURCEDIR)) && (rm -Rf $(SOURCEDIR))
	(test -e $(ROOTDIR)/$(NAME)-$(rpmVersion)*.rpm) && (rm $(ROOTDIR)/$(NAME)-$(rpmVersion)*.rpm)
	(test -e $(executable)) && (rm $(executable))
