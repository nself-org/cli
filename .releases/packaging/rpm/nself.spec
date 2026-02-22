Name:           nself
Version:        0.9.9
Release:        1%{?dist}
Summary:        Self-hosted infrastructure manager for developers

License:        MIT
URL:            https://github.com/acamarata/nself
Source0:        https://github.com/acamarata/nself/archive/v%{version}.tar.gz

BuildArch:      noarch
Requires:       bash, docker, docker-compose, curl, git

%description
nself is a comprehensive CLI tool for deploying and managing
self-hosted backend infrastructure. It provides 36 commands
for managing Docker-based services, SSL certificates, monitoring,
and more. Works on macOS, Linux, and WSL.

%prep
%setup -q

%build
# Nothing to build

%install
rm -rf $RPM_BUILD_ROOT

# Install to /opt
mkdir -p $RPM_BUILD_ROOT/opt/nself
cp -r * $RPM_BUILD_ROOT/opt/nself/

# Create symlink
mkdir -p $RPM_BUILD_ROOT/usr/bin
ln -s /opt/nself/bin/nself $RPM_BUILD_ROOT/usr/bin/nself

# Install documentation
mkdir -p $RPM_BUILD_ROOT/usr/share/doc/nself
cp README.md $RPM_BUILD_ROOT/usr/share/doc/nself/
cp LICENSE $RPM_BUILD_ROOT/usr/share/doc/nself/

%files
%doc README.md
%license LICENSE
/opt/nself/
/usr/bin/nself
/usr/share/doc/nself/

%post
chmod +x /opt/nself/bin/nself
echo "nself v0.9.9 installed successfully!"
echo "Run 'nself help' to get started."

%preun
# Nothing to do

%changelog
* Sat Feb 22 2026 acamarata <contact@acamarata.com> - 0.9.9-1
- Release v0.9.9: Security hardening, CI pipeline fixes, wiki cleanup
- Port binding security: MLflow and PgBouncer now bind to 127.0.0.1
- Added deprecation notices to 12 wiki pages for consolidated commands
- Bash 3.2 compatibility maintained throughout

* Mon Feb 10 2026 acamarata <contact@acamarata.com> - 0.9.8-1
- Release v0.9.8: Production Readiness & Help Contract
- Bash 3.2 compatibility (works on macOS default)
- Help contract across all 31 commands
- CI/CD fail-closed for critical checks
- Published to 5 platforms (Homebrew, npm, Docker Hub, GitHub, AUR)

* Thu Jan 23 2026 acamarata <contact@acamarata.com> - 0.4.7-1
- Release v0.4.7: Kubernetes Support
- New k8s command for Kubernetes operations
- New helm command for Helm chart management
- Cloud provider support for 26 providers
- Full K8s manifest generation from docker-compose
- Bash 3.2 compatibility fixes for all provider files

* Wed Jan 22 2026 acamarata <contact@acamarata.com> - 0.4.6-1
- Release v0.4.6: Scaling & Performance
- New perf, bench, scale, migrate commands
- Performance profiling and load testing
- Cross-environment migration

* Tue Jan 21 2026 acamarata <contact@acamarata.com> - 0.4.5-1
- Release v0.4.5: Provider Support
- New providers command for cloud credential management
- New provision command for one-click infrastructure deployment
- Support for 10 cloud providers (AWS, GCP, Azure, DigitalOcean, Hetzner, etc)
- New sync command for environment synchronization
- New ci command for CI/CD workflow generation

* Mon Jan 20 2026 acamarata <contact@acamarata.com> - 0.4.4-1
- Release v0.4.4: Database Tools
- New db command with comprehensive database management
- DBML schema workflow (scaffold, import, apply)
- Environment-aware seeding and mock data generation
- Type generation for TypeScript, Go, Python

* Sun Jan 19 2026 acamarata <contact@acamarata.com> - 0.4.3-1
- Release v0.4.3: Deployment Pipeline
- New env command for environment management
- Enhanced deploy command with zero-downtime support
- New prod and staging shortcut commands
- Fixed nginx variable substitution and 16 Dockerfile templates

* Sat Jan 18 2026 acamarata <contact@acamarata.com> - 0.4.2-1
- Release v0.4.2: Service & Monitoring Management
- 6 new commands: email, search, functions, mlflow, metrics, monitor
- 92 unit tests, complete documentation

* Fri Jan 17 2026 acamarata <contact@acamarata.com> - 0.4.1-1
- Release v0.4.1: Platform compatibility fixes
- Fixed Bash 3.2 compatibility for macOS
- Fixed cross-platform sed, stat, and timeout commands
- 36 CLI commands for comprehensive infrastructure management

* Sun Oct 13 2025 acamarata <contact@acamarata.com> - 0.4.0-1
- Release v0.4.0: Production-ready release
- All core features complete and tested
- Enhanced cross-platform compatibility

* Sat Aug 17 2024 acamarata <contact@acamarata.com> - 0.3.8-1
- Release v0.3.8: Enterprise features and critical fixes
- Added backup systems, monitoring, and SSL management
- 33 CLI commands for comprehensive infrastructure management
