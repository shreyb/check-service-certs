Name:           check-service-certs
Version:        0.1
Release:        1
Summary:        Simple utility to check X509 certificates for expiration, and send email and Slack notifications if needed 

Group:          Applications/System
License:        Fermitools Software Legal Information (Modified BSD License)
URL:            TODO 
Source0:        %{name}-%{version}.tar.gz

BuildRoot:      %(mktemp -ud %{_tmppath}/%{name}-%{version}-XXXXXX)
BuildArch:      x86_64

%description
Simple utility to check X509 certificates for expiration, and send email and Slack notifications if needed 

%prep
test ! -d %{buildroot} || {
rm -rf %{buildroot}
}

%setup -q

%build

%install
%undefine _missing_build_ids_terminate_build

# Config file to /etc/check-service-certs
mkdir -p %{buildroot}/%{_sysconfdir}/%{name}
install -m 0774 checkServiceCerts.yml %{buildroot}/%{_sysconfdir}/%{name}/checkServiceCerts.yml

# Executable to /usr/bin
mkdir -p %{buildroot}/%{_bindir}
install -m 0755 check-service-certs %{buildroot}/%{_bindir}/check-service-certs

# Cron and logrotate
mkdir -p %{buildroot}/%{_sysconfdir}/cron.d
install -m 0644 %{name}.cron %{buildroot}/%{_sysconfdir}/cron.d/%{name}
mkdir -p %{buildroot}/%{_sysconfdir}/logrotate.d
install -m 0644 %{name}.logrotate %{buildroot}/%{_sysconfdir}/logrotate.d/%{name}

# Templates
mkdir -p %{buildroot}/%{_datadir}/%{name}/templates
install -m 0644 expiringCertificate.txt  %{buildroot}/%{_datadir}/%{name}/templates


%clean
rm -rf %{buildroot}

%files
%defattr(0755, rexbatch, fife, 0774)
%{_sysconfdir}/%{name}
%config(noreplace) %{_sysconfdir}/%{name}/checkServiceCerts.yml
%config(noreplace) %attr(0644, root, root) %{_sysconfdir}/cron.d/%{name}
%config(noreplace) %attr(0644, root, root) %{_sysconfdir}/logrotate.d/%{name}
%{_datadir}/%{name}/templates
%{_bindir}/%{name}

%post
# Set owner of /etc/check-service-certs
test -d %{_sysconfdir}/%{name} && {
chown rexbatch:fife %{_sysconfdir}/%{name}
}

# Logfiles at /var/log/check-service-certs
test -d /var/log/%{name} || {
install -d /var/log/%{name} -m 0774 -o rexbatch -g fife
}


%changelog
* Fri Feb 03 2023 Shreyas Bhat <sbhat@fnal.gov> - 0.1
First version of the check-service-certs RPM
