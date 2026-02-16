#!/usr/bin/env bash
# completion.sh - Shell completion for nself CLI
# Supports bash, zsh, and fish

set -o pipefail

set -euo pipefail


# Source shared utilities
CLI_SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "$CLI_SCRIPT_DIR/../lib/utils/display.sh" 2>/dev/null || true

# Fallback logging
if ! declare -f log_success >/dev/null 2>&1; then
  log_success() { printf "\033[0;32m[SUCCESS]\033[0m %s\n" "$1"; }
fi
if ! declare -f log_info >/dev/null 2>&1; then
  log_info() { printf "\033[0;34m[INFO]\033[0m %s\n" "$1"; }
fi
if ! declare -f log_error >/dev/null 2>&1; then
  log_error() { printf "\033[0;31m[ERROR]\033[0m %s\n" "$1" >&2; }
fi

# ============================================================================
# BASH COMPLETION
# ============================================================================

generate_bash() {
  cat <<'BASH_COMPLETION'
# nself bash completion
# Add to ~/.bashrc: eval "$(nself completion bash)"

_nself_completions() {
    local cur prev words cword
    _init_completion || return

    # Main commands (v0.9.7)
    local commands="init build start stop restart status logs exec urls doctor help update ssl trust admin clean reset version env deploy prod staging db sync provider cloud service k8s helm perf bench scale migrate health frontend history config plugin ci completion tenant monitor restore rollback validate upgrade"

    # Subcommands by parent
    local db_commands="migrate seed mock backup restore schema types shell query inspect data optimize reset status help"
    local env_commands="create switch list diff validate export import"
    local sync_commands="db files config full auto watch status history"
    local ssl_commands="generate renew bootstrap check trust"
    local deploy_commands="staging production rollback preview canary blue-green"
    local tenant_commands="init create list show delete switch suspend activate billing branding domains email themes"
    local monitor_commands="start stop status logs grafana prometheus alertmanager"
    local plugin_commands="list install remove update status"

    # v0.9.6 provider command subcommands
    local provider_commands="info server cost deploy list init validate show create destroy ssh"
    local provider_info_commands="list init validate show"
    local provider_server_commands="create destroy list status ssh add remove"
    local provider_cost_commands="estimate compare"
    local provider_deploy_commands="quick full"

    # v0.4.7 cloud command (deprecated, maps to provider)
    local cloud_commands="provider server cost deploy"
    local cloud_provider_commands="list init validate info"
    local cloud_server_commands="create destroy list status ssh add remove"
    local cloud_cost_commands="estimate compare"
    local cloud_deploy_commands="quick full"

    local service_commands="list enable disable status restart logs email search functions mlflow storage cache"
    local service_email_commands="test inbox config"
    local service_search_commands="index query stats"
    local service_functions_commands="deploy invoke logs list"
    local service_mlflow_commands="ui experiments runs artifacts"
    local service_storage_commands="buckets upload download presign"
    local service_cache_commands="stats flush keys"

    local k8s_commands="init convert apply deploy status logs scale rollback delete cluster namespace"
    local k8s_cluster_commands="list connect info"
    local k8s_namespace_commands="list create delete switch"

    local helm_commands="init generate install upgrade rollback uninstall list status values template package repo"
    local helm_repo_commands="add remove update list"

    local perf_commands="profile analyze benchmark report compare"
    local bench_commands="run http db compare report"
    local scale_commands="up down auto status config"
    local health_commands="check dashboard alert history"

    # Plugin commands and available plugins
    local plugin_names="stripe github shopify"
    local plugin_actions="sync customers subscriptions invoices webhook repos issues prs workflows products orders"

    # Frontend commands
    local frontend_commands="add remove list status config"

    # History commands
    local history_commands="list show stats clear filter"

    # Config commands
    local config_commands="show edit validate diff backup restore"

    case "${prev}" in
        nself)
            COMPREPLY=($(compgen -W "${commands}" -- "${cur}"))
            return 0
            ;;
        db)
            COMPREPLY=($(compgen -W "${db_commands}" -- "${cur}"))
            return 0
            ;;
        env)
            COMPREPLY=($(compgen -W "${env_commands}" -- "${cur}"))
            return 0
            ;;
        sync)
            COMPREPLY=($(compgen -W "${sync_commands}" -- "${cur}"))
            return 0
            ;;
        ssl)
            COMPREPLY=($(compgen -W "${ssl_commands}" -- "${cur}"))
            return 0
            ;;
        deploy)
            COMPREPLY=($(compgen -W "${deploy_commands}" -- "${cur}"))
            return 0
            ;;
        provider)
            COMPREPLY=($(compgen -W "${provider_commands}" -- "${cur}"))
            return 0
            ;;
        cloud)
            COMPREPLY=($(compgen -W "${cloud_commands}" -- "${cur}"))
            return 0
            ;;
        info)
            COMPREPLY=($(compgen -W "${provider_info_commands}" -- "${cur}"))
            return 0
            ;;
        server)
            COMPREPLY=($(compgen -W "${provider_server_commands}" -- "${cur}"))
            return 0
            ;;
        cost)
            COMPREPLY=($(compgen -W "${provider_cost_commands}" -- "${cur}"))
            return 0
            ;;
        service)
            COMPREPLY=($(compgen -W "${service_commands}" -- "${cur}"))
            return 0
            ;;
        email)
            COMPREPLY=($(compgen -W "${service_email_commands}" -- "${cur}"))
            return 0
            ;;
        search)
            COMPREPLY=($(compgen -W "${service_search_commands}" -- "${cur}"))
            return 0
            ;;
        functions)
            COMPREPLY=($(compgen -W "${service_functions_commands}" -- "${cur}"))
            return 0
            ;;
        mlflow)
            COMPREPLY=($(compgen -W "${service_mlflow_commands}" -- "${cur}"))
            return 0
            ;;
        storage)
            COMPREPLY=($(compgen -W "${service_storage_commands}" -- "${cur}"))
            return 0
            ;;
        cache)
            COMPREPLY=($(compgen -W "${service_cache_commands}" -- "${cur}"))
            return 0
            ;;
        k8s)
            COMPREPLY=($(compgen -W "${k8s_commands}" -- "${cur}"))
            return 0
            ;;
        cluster)
            COMPREPLY=($(compgen -W "${k8s_cluster_commands}" -- "${cur}"))
            return 0
            ;;
        namespace)
            COMPREPLY=($(compgen -W "${k8s_namespace_commands}" -- "${cur}"))
            return 0
            ;;
        helm)
            COMPREPLY=($(compgen -W "${helm_commands}" -- "${cur}"))
            return 0
            ;;
        repo)
            COMPREPLY=($(compgen -W "${helm_repo_commands}" -- "${cur}"))
            return 0
            ;;
        perf)
            COMPREPLY=($(compgen -W "${perf_commands}" -- "${cur}"))
            return 0
            ;;
        bench)
            COMPREPLY=($(compgen -W "${bench_commands}" -- "${cur}"))
            return 0
            ;;
        scale)
            COMPREPLY=($(compgen -W "${scale_commands}" -- "${cur}"))
            return 0
            ;;
        health)
            COMPREPLY=($(compgen -W "${health_commands}" -- "${cur}"))
            return 0
            ;;
        plugin)
            COMPREPLY=($(compgen -W "${plugin_commands} ${plugin_names}" -- "${cur}"))
            return 0
            ;;
        tenant)
            COMPREPLY=($(compgen -W "${tenant_commands}" -- "${cur}"))
            return 0
            ;;
        monitor)
            COMPREPLY=($(compgen -W "${monitor_commands}" -- "${cur}"))
            return 0
            ;;
        frontend)
            COMPREPLY=($(compgen -W "${frontend_commands}" -- "${cur}"))
            return 0
            ;;
        history)
            COMPREPLY=($(compgen -W "${history_commands}" -- "${cur}"))
            return 0
            ;;
        config)
            COMPREPLY=($(compgen -W "${config_commands}" -- "${cur}"))
            return 0
            ;;
        stripe)
            COMPREPLY=($(compgen -W "sync customers subscriptions invoices webhook" -- "${cur}"))
            return 0
            ;;
        github)
            COMPREPLY=($(compgen -W "sync repos issues prs workflows webhook" -- "${cur}"))
            return 0
            ;;
        shopify)
            COMPREPLY=($(compgen -W "sync products orders customers webhook" -- "${cur}"))
            return 0
            ;;
        migrate)
            COMPREPLY=($(compgen -W "status up down create fresh repair" -- "${cur}"))
            return 0
            ;;
        seed)
            COMPREPLY=($(compgen -W "run users create status" -- "${cur}"))
            return 0
            ;;
        mock)
            COMPREPLY=($(compgen -W "generate auto preview clear config" -- "${cur}"))
            return 0
            ;;
        backup)
            COMPREPLY=($(compgen -W "create list restore schedule prune" -- "${cur}"))
            return 0
            ;;
        schema)
            COMPREPLY=($(compgen -W "show diff diagram indexes export import scaffold apply" -- "${cur}"))
            return 0
            ;;
        types)
            COMPREPLY=($(compgen -W "typescript go python" -- "${cur}"))
            return 0
            ;;
        inspect)
            COMPREPLY=($(compgen -W "overview size cache index unused-indexes bloat slow locks connections" -- "${cur}"))
            return 0
            ;;
        data)
            COMPREPLY=($(compgen -W "export import anonymize sync" -- "${cur}"))
            return 0
            ;;
        pull|push)
            COMPREPLY=($(compgen -W "staging production" -- "${cur}"))
            return 0
            ;;
        files|config)
            COMPREPLY=($(compgen -W "pull push diff" -- "${cur}"))
            return 0
            ;;
        doctor)
            COMPREPLY=($(compgen -W "--fix --verbose --help" -- "${cur}"))
            return 0
            ;;
        scaffold)
            COMPREPLY=($(compgen -W "basic ecommerce saas blog" -- "${cur}"))
            return 0
            ;;
        canary)
            COMPREPLY=($(compgen -W "promote rollback status" -- "${cur}"))
            return 0
            ;;
        blue-green)
            COMPREPLY=($(compgen -W "switch rollback status" -- "${cur}"))
            return 0
            ;;
        billing)
            COMPREPLY=($(compgen -W "usage invoices quotas plans upgrade downgrade" -- "${cur}"))
            return 0
            ;;
        branding)
            COMPREPLY=($(compgen -W "logo colors themes css preview" -- "${cur}"))
            return 0
            ;;
        domains)
            COMPREPLY=($(compgen -W "add remove list verify ssl status" -- "${cur}"))
            return 0
            ;;
        themes)
            COMPREPLY=($(compgen -W "create edit preview activate list" -- "${cur}"))
            return 0
            ;;
        create)
            # Provider names for cloud server create
            local providers="digitalocean linode vultr hetzner ovh scaleway upcloud aws gcp azure oracle ibm contabo hostinger kamatera ssdnodes exoscale alibaba tencent yandex racknerd buyvm time4vps raspberrypi custom"
            COMPREPLY=($(compgen -W "${providers}" -- "${cur}"))
            return 0
            ;;
        logs)
            # Complete with service names from docker-compose
            if [[ -f "docker-compose.yml" ]]; then
                local services=$(grep -E "^  [a-z].*:" docker-compose.yml 2>/dev/null | sed 's/://g' | xargs)
                COMPREPLY=($(compgen -W "${services}" -- "${cur}"))
            fi
            return 0
            ;;
        exec)
            # Complete with service names
            if [[ -f "docker-compose.yml" ]]; then
                local services=$(grep -E "^  [a-z].*:" docker-compose.yml 2>/dev/null | sed 's/://g' | xargs)
                COMPREPLY=($(compgen -W "${services}" -- "${cur}"))
            fi
            return 0
            ;;
    esac

    # Default: show main commands
    if [[ ${cword} -eq 1 ]]; then
        COMPREPLY=($(compgen -W "${commands}" -- "${cur}"))
    fi

    return 0
}

complete -F _nself_completions nself
BASH_COMPLETION
}

# ============================================================================
# ZSH COMPLETION
# ============================================================================

generate_zsh() {
  cat <<'ZSH_COMPLETION'
#compdef nself
# nself zsh completion (v0.9.7)
# Add to ~/.zshrc: eval "$(nself completion zsh)"

_nself() {
    local -a commands db_commands env_commands sync_commands deploy_commands
    local -a cloud_commands service_commands k8s_commands helm_commands
    local -a perf_commands bench_commands scale_commands health_commands
    local -a tenant_commands monitor_commands plugin_commands frontend_commands
    local -a history_commands config_commands provider_commands

    commands=(
        'init:Initialize a new nself project'
        'build:Build project configuration and containers'
        'start:Start all services'
        'stop:Stop all services'
        'restart:Restart all services'
        'status:Show service status'
        'logs:View service logs'
        'exec:Execute command in service container'
        'urls:Show all service URLs'
        'doctor:Run system diagnostics'
        'help:Show help information'
        'update:Update nself CLI and admin'
        'ssl:SSL certificate management'
        'trust:Install SSL root CA to system'
        'admin:Open admin dashboard'
        'clean:Clean up containers and volumes'
        'reset:Reset project to initial state'
        'version:Show version information'
        'env:Environment management'
        'deploy:Deploy with advanced strategies'
        'prod:Production deployment shortcut'
        'staging:Staging deployment shortcut'
        'db:Database management'
        'sync:Environment synchronization with auto-watch'
        'provider:Provider infrastructure management'
        'cloud:DEPRECATED - Use provider instead'
        'service:Optional service management'
        'k8s:Kubernetes management'
        'helm:Helm chart management'
        'perf:Performance profiling'
        'bench:Benchmarking and load testing'
        'scale:Service scaling'
        'migrate:Cross-environment migration'
        'health:Health check management'
        'frontend:Frontend application management'
        'history:Deployment audit trail'
        'config:Configuration management'
        'plugin:Plugin management'
        'tenant:Multi-tenant management'
        'ci:CI/CD workflow generation'
        'completion:Generate shell completions'
        'monitor:Monitoring dashboard management'
        'restore:Restore configuration from backup'
        'rollback:Rollback deployments or migrations'
        'validate:Validate configuration files'
        'upgrade:Upgrade nself or dependencies'
    )

    db_commands=(
        'migrate:Run database migrations'
        'seed:Seed database with data'
        'mock:Generate mock data'
        'backup:Create database backup'
        'restore:Restore from backup'
        'schema:Schema management tools'
        'types:Generate type definitions'
        'shell:Open database shell'
        'query:Execute SQL query'
        'inspect:Database analysis'
        'data:Data import/export'
        'optimize:Optimize database'
        'reset:Reset database'
        'status:Show database status'
    )

    env_commands=(
        'create:Create new environment'
        'switch:Switch to environment'
        'list:List environments'
        'diff:Compare environments'
        'validate:Validate configuration'
        'export:Export environment config'
        'import:Import environment config'
    )

    sync_commands=(
        'db:Sync database'
        'files:Sync files'
        'config:Sync configuration'
        'full:Full environment sync'
        'auto:Enable continuous auto-sync'
        'watch:Manual file watch mode'
        'status:Show sync status'
        'history:Show sync history'
    )

    deploy_commands=(
        'staging:Deploy to staging'
        'production:Deploy to production'
        'rollback:Rollback deployment'
        'preview:Create preview environment'
        'canary:Canary deployment'
        'blue-green:Blue-green deployment'
    )

    provider_commands=(
        'info:Provider information (list, init, validate, show)'
        'server:Server management (create, destroy, list, status, ssh)'
        'cost:Cost estimation (estimate, compare)'
        'deploy:Quick deployment (quick, full)'
        'list:List all providers (shortcut)'
        'init:Initialize provider (shortcut)'
        'validate:Validate configuration (shortcut)'
        'show:Show provider info (shortcut)'
        'create:Create server (shortcut)'
        'destroy:Destroy server (shortcut)'
        'ssh:SSH to server (shortcut)'
    )

    cloud_commands=(
        'provider:Provider management (list, init, validate, info)'
        'server:Server management (create, destroy, list, status, ssh)'
        'cost:Cost estimation (estimate, compare)'
        'deploy:Quick deployment (quick, full)'
    )

    service_commands=(
        'list:List all optional services'
        'enable:Enable a service'
        'disable:Disable a service'
        'status:Show service status'
        'restart:Restart a service'
        'logs:View service logs'
        'email:Email service management'
        'search:Search service management'
        'functions:Serverless functions'
        'mlflow:ML experiment tracking'
        'storage:Object storage (MinIO)'
        'cache:Cache management (Redis)'
    )

    k8s_commands=(
        'init:Initialize Kubernetes configuration'
        'convert:Convert compose to K8s manifests'
        'apply:Apply manifests to cluster'
        'deploy:Deploy to Kubernetes'
        'status:Show deployment status'
        'logs:View pod logs'
        'scale:Scale deployment'
        'rollback:Rollback deployment'
        'delete:Delete deployment'
        'cluster:Cluster management'
        'namespace:Namespace management'
    )

    helm_commands=(
        'init:Initialize Helm configuration'
        'generate:Generate Helm chart from compose'
        'install:Install Helm release'
        'upgrade:Upgrade Helm release'
        'rollback:Rollback Helm release'
        'uninstall:Uninstall Helm release'
        'list:List Helm releases'
        'status:Show release status'
        'values:Show/set chart values'
        'template:Render chart templates'
        'package:Package Helm chart'
        'repo:Repository management'
    )

    perf_commands=(
        'profile:Profile application performance'
        'analyze:Analyze performance data'
        'benchmark:Run benchmarks'
        'report:Generate performance report'
        'compare:Compare performance runs'
    )

    bench_commands=(
        'run:Run benchmarks'
        'http:HTTP load testing'
        'db:Database benchmarks'
        'compare:Compare benchmark results'
        'report:Generate benchmark report'
    )

    scale_commands=(
        'up:Scale up services'
        'down:Scale down services'
        'auto:Configure autoscaling'
        'status:Show scaling status'
        'config:Configure scaling rules'
    )

    health_commands=(
        'check:Run health checks'
        'dashboard:Open health dashboard'
        'alert:Configure health alerts'
        'history:Show health history'
    )

    tenant_commands=(
        'init:Initialize multi-tenancy'
        'create:Create new tenant'
        'list:List all tenants'
        'show:Show tenant details'
        'delete:Delete tenant'
        'switch:Switch active tenant'
        'suspend:Suspend tenant'
        'activate:Activate tenant'
        'billing:Billing management'
        'branding:Branding customization'
        'domains:Domain management'
        'email:Email template management'
        'themes:Theme management'
    )

    monitor_commands=(
        'start:Start monitoring stack'
        'stop:Stop monitoring stack'
        'status:Show monitoring status'
        'logs:View monitoring logs'
        'grafana:Open Grafana dashboard'
        'prometheus:Open Prometheus UI'
        'alertmanager:Open Alertmanager UI'
    )

    plugin_commands=(
        'list:List available plugins'
        'install:Install plugin'
        'remove:Remove plugin'
        'update:Update plugin'
        'status:Show plugin status'
    )

    frontend_commands=(
        'add:Add frontend application'
        'remove:Remove frontend application'
        'list:List frontend applications'
        'status:Show frontend status'
        'config:Configure frontend'
    )

    history_commands=(
        'list:List deployment history'
        'show:Show deployment details'
        'stats:Show deployment statistics'
        'clear:Clear history'
        'filter:Filter history entries'
    )

    config_commands=(
        'show:Show configuration'
        'edit:Edit configuration'
        'validate:Validate configuration'
        'diff:Compare configurations'
        'backup:Backup configuration'
        'restore:Restore configuration'
    )

    _arguments -C \
        '1: :->command' \
        '2: :->subcommand' \
        '3: :->subsubcommand' \
        '*::arg:->args'

    case "$state" in
        command)
            _describe -t commands 'nself command' commands
            ;;
        subcommand)
            case "$words[2]" in
                db)
                    _describe -t db_commands 'db command' db_commands
                    ;;
                env)
                    _describe -t env_commands 'env command' env_commands
                    ;;
                sync)
                    _describe -t sync_commands 'sync command' sync_commands
                    ;;
                deploy)
                    _describe -t deploy_commands 'deploy command' deploy_commands
                    ;;
                provider)
                    _describe -t provider_commands 'provider command' provider_commands
                    ;;
                cloud)
                    _describe -t cloud_commands 'cloud command (deprecated)' cloud_commands
                    ;;
                service)
                    _describe -t service_commands 'service command' service_commands
                    ;;
                k8s)
                    _describe -t k8s_commands 'k8s command' k8s_commands
                    ;;
                helm)
                    _describe -t helm_commands 'helm command' helm_commands
                    ;;
                perf)
                    _describe -t perf_commands 'perf command' perf_commands
                    ;;
                bench)
                    _describe -t bench_commands 'bench command' bench_commands
                    ;;
                scale)
                    _describe -t scale_commands 'scale command' scale_commands
                    ;;
                health)
                    _describe -t health_commands 'health command' health_commands
                    ;;
                tenant)
                    _describe -t tenant_commands 'tenant command' tenant_commands
                    ;;
                monitor)
                    _describe -t monitor_commands 'monitor command' monitor_commands
                    ;;
                plugin)
                    _describe -t plugin_commands 'plugin command' plugin_commands
                    ;;
                frontend)
                    _describe -t frontend_commands 'frontend command' frontend_commands
                    ;;
                history)
                    _describe -t history_commands 'history command' history_commands
                    ;;
                config)
                    _describe -t config_commands 'config command' config_commands
                    ;;
                logs|exec)
                    if [[ -f "docker-compose.yml" ]]; then
                        local services
                        services=($(grep -E "^  [a-z].*:" docker-compose.yml 2>/dev/null | sed 's/://g'))
                        _describe -t services 'service' services
                    fi
                    ;;
                doctor)
                    local -a doctor_opts
                    doctor_opts=('--fix:Auto-fix detected issues' '--verbose:Verbose output' '--help:Show help')
                    _describe -t doctor_opts 'doctor option' doctor_opts
                    ;;
            esac
            ;;
        subsubcommand)
            case "$words[2]-$words[3]" in
                provider-info)
                    local -a info_cmds=('list:List available providers' 'init:Initialize provider' 'validate:Validate configuration' 'show:Show provider info')
                    _describe -t info_cmds 'info command' info_cmds
                    ;;
                provider-server)
                    local -a server_cmds=('create:Create server' 'destroy:Destroy server' 'list:List servers' 'status:Show status' 'ssh:SSH to server' 'add:Add existing server' 'remove:Remove server')
                    _describe -t server_cmds 'server command' server_cmds
                    ;;
                provider-cost)
                    local -a cost_cmds=('estimate:Estimate costs' 'compare:Compare providers')
                    _describe -t cost_cmds 'cost command' cost_cmds
                    ;;
                cloud-provider)
                    local -a provider_cmds=('list:List available providers' 'init:Initialize provider' 'validate:Validate configuration' 'info:Show provider info')
                    _describe -t provider_cmds 'provider command (deprecated)' provider_cmds
                    ;;
                cloud-server)
                    local -a server_cmds=('create:Create server' 'destroy:Destroy server' 'list:List servers' 'status:Show status' 'ssh:SSH to server' 'add:Add existing server' 'remove:Remove server')
                    _describe -t server_cmds 'server command (deprecated)' server_cmds
                    ;;
                cloud-cost)
                    local -a cost_cmds=('estimate:Estimate costs' 'compare:Compare providers')
                    _describe -t cost_cmds 'cost command (deprecated)' cost_cmds
                    ;;
                k8s-cluster)
                    local -a cluster_cmds=('list:List clusters' 'connect:Connect to cluster' 'info:Show cluster info')
                    _describe -t cluster_cmds 'cluster command' cluster_cmds
                    ;;
                k8s-namespace)
                    local -a ns_cmds=('list:List namespaces' 'create:Create namespace' 'delete:Delete namespace' 'switch:Switch namespace')
                    _describe -t ns_cmds 'namespace command' ns_cmds
                    ;;
                helm-repo)
                    local -a repo_cmds=('add:Add repository' 'remove:Remove repository' 'update:Update repositories' 'list:List repositories')
                    _describe -t repo_cmds 'repo command' repo_cmds
                    ;;
                service-email)
                    local -a email_cmds=('test:Send test email' 'inbox:View inbox (MailPit)' 'config:Email configuration')
                    _describe -t email_cmds 'email command' email_cmds
                    ;;
                service-search)
                    local -a search_cmds=('index:Reindex data' 'query:Run search query' 'stats:Show statistics')
                    _describe -t search_cmds 'search command' search_cmds
                    ;;
                service-functions)
                    local -a func_cmds=('deploy:Deploy function' 'invoke:Invoke function' 'logs:View logs' 'list:List functions')
                    _describe -t func_cmds 'functions command' func_cmds
                    ;;
                deploy-canary)
                    local -a canary_cmds=('promote:Promote canary' 'rollback:Rollback canary' 'status:Show status')
                    _describe -t canary_cmds 'canary command' canary_cmds
                    ;;
                deploy-blue-green)
                    local -a bg_cmds=('switch:Switch traffic' 'rollback:Rollback' 'status:Show status')
                    _describe -t bg_cmds 'blue-green command' bg_cmds
                    ;;
                tenant-billing)
                    local -a billing_cmds=('usage:Usage tracking' 'invoices:Invoice management' 'quotas:Quota management' 'plans:Plan management' 'upgrade:Upgrade plan' 'downgrade:Downgrade plan')
                    _describe -t billing_cmds 'billing command' billing_cmds
                    ;;
                tenant-branding)
                    local -a branding_cmds=('logo:Logo management' 'colors:Color customization' 'themes:Theme selection' 'css:Custom CSS' 'preview:Preview branding')
                    _describe -t branding_cmds 'branding command' branding_cmds
                    ;;
                tenant-domains)
                    local -a domain_cmds=('add:Add domain' 'remove:Remove domain' 'list:List domains' 'verify:Verify domain' 'ssl:SSL management' 'status:Domain status')
                    _describe -t domain_cmds 'domain command' domain_cmds
                    ;;
                tenant-themes)
                    local -a theme_cmds=('create:Create theme' 'edit:Edit theme' 'preview:Preview theme' 'activate:Activate theme' 'list:List themes')
                    _describe -t theme_cmds 'theme command' theme_cmds
                    ;;
            esac
            ;;
    esac
}

_nself "$@"
ZSH_COMPLETION
}

# ============================================================================
# FISH COMPLETION
# ============================================================================

generate_fish() {
  cat <<'FISH_COMPLETION'
# nself fish completion (v0.9.7)
# Save to ~/.config/fish/completions/nself.fish

# Main commands
complete -c nself -f -n "__fish_use_subcommand" -a "init" -d "Initialize new project"
complete -c nself -f -n "__fish_use_subcommand" -a "build" -d "Build configuration"
complete -c nself -f -n "__fish_use_subcommand" -a "start" -d "Start services"
complete -c nself -f -n "__fish_use_subcommand" -a "stop" -d "Stop services"
complete -c nself -f -n "__fish_use_subcommand" -a "restart" -d "Restart services"
complete -c nself -f -n "__fish_use_subcommand" -a "status" -d "Show status"
complete -c nself -f -n "__fish_use_subcommand" -a "logs" -d "View logs"
complete -c nself -f -n "__fish_use_subcommand" -a "exec" -d "Execute command"
complete -c nself -f -n "__fish_use_subcommand" -a "urls" -d "Show URLs"
complete -c nself -f -n "__fish_use_subcommand" -a "doctor" -d "System diagnostics"
complete -c nself -f -n "__fish_use_subcommand" -a "help" -d "Show help"
complete -c nself -f -n "__fish_use_subcommand" -a "update" -d "Update CLI"
complete -c nself -f -n "__fish_use_subcommand" -a "ssl" -d "SSL management"
complete -c nself -f -n "__fish_use_subcommand" -a "trust" -d "Install root CA"
complete -c nself -f -n "__fish_use_subcommand" -a "admin" -d "Admin dashboard"
complete -c nself -f -n "__fish_use_subcommand" -a "clean" -d "Clean up"
complete -c nself -f -n "__fish_use_subcommand" -a "reset" -d "Reset project"
complete -c nself -f -n "__fish_use_subcommand" -a "version" -d "Show version"
complete -c nself -f -n "__fish_use_subcommand" -a "env" -d "Environment management"
complete -c nself -f -n "__fish_use_subcommand" -a "deploy" -d "Deploy with strategies"
complete -c nself -f -n "__fish_use_subcommand" -a "prod" -d "Production deploy"
complete -c nself -f -n "__fish_use_subcommand" -a "staging" -d "Staging deploy"
complete -c nself -f -n "__fish_use_subcommand" -a "db" -d "Database management"
complete -c nself -f -n "__fish_use_subcommand" -a "sync" -d "Environment sync"
complete -c nself -f -n "__fish_use_subcommand" -a "provider" -d "Provider infrastructure"
complete -c nself -f -n "__fish_use_subcommand" -a "cloud" -d "DEPRECATED - Use provider"
complete -c nself -f -n "__fish_use_subcommand" -a "service" -d "Optional services"
complete -c nself -f -n "__fish_use_subcommand" -a "k8s" -d "Kubernetes management"
complete -c nself -f -n "__fish_use_subcommand" -a "helm" -d "Helm chart management"
complete -c nself -f -n "__fish_use_subcommand" -a "perf" -d "Performance profiling"
complete -c nself -f -n "__fish_use_subcommand" -a "bench" -d "Benchmarking"
complete -c nself -f -n "__fish_use_subcommand" -a "scale" -d "Service scaling"
complete -c nself -f -n "__fish_use_subcommand" -a "migrate" -d "Cross-env migration"
complete -c nself -f -n "__fish_use_subcommand" -a "health" -d "Health checks"
complete -c nself -f -n "__fish_use_subcommand" -a "frontend" -d "Frontend apps"
complete -c nself -f -n "__fish_use_subcommand" -a "history" -d "Audit trail"
complete -c nself -f -n "__fish_use_subcommand" -a "config" -d "Configuration"
complete -c nself -f -n "__fish_use_subcommand" -a "ci" -d "CI/CD workflows"
complete -c nself -f -n "__fish_use_subcommand" -a "completion" -d "Shell completions"
complete -c nself -f -n "__fish_use_subcommand" -a "tenant" -d "Multi-tenant management"
complete -c nself -f -n "__fish_use_subcommand" -a "monitor" -d "Monitoring dashboards"
complete -c nself -f -n "__fish_use_subcommand" -a "restore" -d "Restore configuration"
complete -c nself -f -n "__fish_use_subcommand" -a "rollback" -d "Rollback deployments"
complete -c nself -f -n "__fish_use_subcommand" -a "validate" -d "Validate configuration"
complete -c nself -f -n "__fish_use_subcommand" -a "upgrade" -d "Upgrade nself"
complete -c nself -f -n "__fish_use_subcommand" -a "plugin" -d "Plugin management"

# db subcommands
complete -c nself -f -n "__fish_seen_subcommand_from db" -a "migrate" -d "Run migrations"
complete -c nself -f -n "__fish_seen_subcommand_from db" -a "seed" -d "Seed data"
complete -c nself -f -n "__fish_seen_subcommand_from db" -a "mock" -d "Generate mock data"
complete -c nself -f -n "__fish_seen_subcommand_from db" -a "backup" -d "Create backup"
complete -c nself -f -n "__fish_seen_subcommand_from db" -a "restore" -d "Restore backup"
complete -c nself -f -n "__fish_seen_subcommand_from db" -a "schema" -d "Schema tools"
complete -c nself -f -n "__fish_seen_subcommand_from db" -a "types" -d "Generate types"
complete -c nself -f -n "__fish_seen_subcommand_from db" -a "shell" -d "Database shell"
complete -c nself -f -n "__fish_seen_subcommand_from db" -a "query" -d "Execute query"
complete -c nself -f -n "__fish_seen_subcommand_from db" -a "inspect" -d "Analyze database"
complete -c nself -f -n "__fish_seen_subcommand_from db" -a "data" -d "Import/export data"
complete -c nself -f -n "__fish_seen_subcommand_from db" -a "optimize" -d "Optimize database"
complete -c nself -f -n "__fish_seen_subcommand_from db" -a "reset" -d "Reset database"

# sync subcommands
complete -c nself -f -n "__fish_seen_subcommand_from sync" -a "db" -d "Sync database"
complete -c nself -f -n "__fish_seen_subcommand_from sync" -a "files" -d "Sync files"
complete -c nself -f -n "__fish_seen_subcommand_from sync" -a "config" -d "Sync config"
complete -c nself -f -n "__fish_seen_subcommand_from sync" -a "full" -d "Full sync"
complete -c nself -f -n "__fish_seen_subcommand_from sync" -a "auto" -d "Auto-sync mode"
complete -c nself -f -n "__fish_seen_subcommand_from sync" -a "watch" -d "Watch mode"
complete -c nself -f -n "__fish_seen_subcommand_from sync" -a "status" -d "Connection status"
complete -c nself -f -n "__fish_seen_subcommand_from sync" -a "history" -d "Sync history"

# deploy subcommands
complete -c nself -f -n "__fish_seen_subcommand_from deploy" -a "staging" -d "Deploy to staging"
complete -c nself -f -n "__fish_seen_subcommand_from deploy" -a "production" -d "Deploy to production"
complete -c nself -f -n "__fish_seen_subcommand_from deploy" -a "rollback" -d "Rollback deployment"
complete -c nself -f -n "__fish_seen_subcommand_from deploy" -a "preview" -d "Preview environment"
complete -c nself -f -n "__fish_seen_subcommand_from deploy" -a "canary" -d "Canary deployment"
complete -c nself -f -n "__fish_seen_subcommand_from deploy" -a "blue-green" -d "Blue-green deploy"

# provider subcommands
complete -c nself -f -n "__fish_seen_subcommand_from provider" -a "info" -d "Provider information"
complete -c nself -f -n "__fish_seen_subcommand_from provider" -a "server" -d "Server management"
complete -c nself -f -n "__fish_seen_subcommand_from provider" -a "cost" -d "Cost estimation"
complete -c nself -f -n "__fish_seen_subcommand_from provider" -a "deploy" -d "Quick deployment"
complete -c nself -f -n "__fish_seen_subcommand_from provider" -a "list" -d "List providers (shortcut)"
complete -c nself -f -n "__fish_seen_subcommand_from provider" -a "init" -d "Init provider (shortcut)"
complete -c nself -f -n "__fish_seen_subcommand_from provider" -a "validate" -d "Validate config (shortcut)"
complete -c nself -f -n "__fish_seen_subcommand_from provider" -a "show" -d "Show info (shortcut)"
complete -c nself -f -n "__fish_seen_subcommand_from provider" -a "create" -d "Create server (shortcut)"
complete -c nself -f -n "__fish_seen_subcommand_from provider" -a "destroy" -d "Destroy server (shortcut)"
complete -c nself -f -n "__fish_seen_subcommand_from provider" -a "ssh" -d "SSH to server (shortcut)"

# cloud subcommands (deprecated)
complete -c nself -f -n "__fish_seen_subcommand_from cloud" -a "provider" -d "Provider management"
complete -c nself -f -n "__fish_seen_subcommand_from cloud" -a "server" -d "Server management"
complete -c nself -f -n "__fish_seen_subcommand_from cloud" -a "cost" -d "Cost estimation"
complete -c nself -f -n "__fish_seen_subcommand_from cloud" -a "deploy" -d "Quick deployment"

# service subcommands
complete -c nself -f -n "__fish_seen_subcommand_from service" -a "list" -d "List services"
complete -c nself -f -n "__fish_seen_subcommand_from service" -a "enable" -d "Enable service"
complete -c nself -f -n "__fish_seen_subcommand_from service" -a "disable" -d "Disable service"
complete -c nself -f -n "__fish_seen_subcommand_from service" -a "status" -d "Service status"
complete -c nself -f -n "__fish_seen_subcommand_from service" -a "restart" -d "Restart service"
complete -c nself -f -n "__fish_seen_subcommand_from service" -a "logs" -d "Service logs"
complete -c nself -f -n "__fish_seen_subcommand_from service" -a "email" -d "Email service"
complete -c nself -f -n "__fish_seen_subcommand_from service" -a "search" -d "Search service"
complete -c nself -f -n "__fish_seen_subcommand_from service" -a "functions" -d "Serverless functions"
complete -c nself -f -n "__fish_seen_subcommand_from service" -a "mlflow" -d "ML tracking"
complete -c nself -f -n "__fish_seen_subcommand_from service" -a "storage" -d "Object storage"
complete -c nself -f -n "__fish_seen_subcommand_from service" -a "cache" -d "Cache (Redis)"

# k8s subcommands
complete -c nself -f -n "__fish_seen_subcommand_from k8s" -a "init" -d "Initialize K8s"
complete -c nself -f -n "__fish_seen_subcommand_from k8s" -a "convert" -d "Convert to manifests"
complete -c nself -f -n "__fish_seen_subcommand_from k8s" -a "apply" -d "Apply manifests"
complete -c nself -f -n "__fish_seen_subcommand_from k8s" -a "deploy" -d "Deploy to cluster"
complete -c nself -f -n "__fish_seen_subcommand_from k8s" -a "status" -d "Deployment status"
complete -c nself -f -n "__fish_seen_subcommand_from k8s" -a "logs" -d "Pod logs"
complete -c nself -f -n "__fish_seen_subcommand_from k8s" -a "scale" -d "Scale deployment"
complete -c nself -f -n "__fish_seen_subcommand_from k8s" -a "rollback" -d "Rollback deployment"
complete -c nself -f -n "__fish_seen_subcommand_from k8s" -a "delete" -d "Delete deployment"
complete -c nself -f -n "__fish_seen_subcommand_from k8s" -a "cluster" -d "Cluster management"
complete -c nself -f -n "__fish_seen_subcommand_from k8s" -a "namespace" -d "Namespace management"

# helm subcommands
complete -c nself -f -n "__fish_seen_subcommand_from helm" -a "init" -d "Initialize Helm"
complete -c nself -f -n "__fish_seen_subcommand_from helm" -a "generate" -d "Generate chart"
complete -c nself -f -n "__fish_seen_subcommand_from helm" -a "install" -d "Install release"
complete -c nself -f -n "__fish_seen_subcommand_from helm" -a "upgrade" -d "Upgrade release"
complete -c nself -f -n "__fish_seen_subcommand_from helm" -a "rollback" -d "Rollback release"
complete -c nself -f -n "__fish_seen_subcommand_from helm" -a "uninstall" -d "Uninstall release"
complete -c nself -f -n "__fish_seen_subcommand_from helm" -a "list" -d "List releases"
complete -c nself -f -n "__fish_seen_subcommand_from helm" -a "status" -d "Release status"
complete -c nself -f -n "__fish_seen_subcommand_from helm" -a "values" -d "Chart values"
complete -c nself -f -n "__fish_seen_subcommand_from helm" -a "template" -d "Render templates"
complete -c nself -f -n "__fish_seen_subcommand_from helm" -a "package" -d "Package chart"
complete -c nself -f -n "__fish_seen_subcommand_from helm" -a "repo" -d "Repository management"

# perf subcommands
complete -c nself -f -n "__fish_seen_subcommand_from perf" -a "profile" -d "Profile performance"
complete -c nself -f -n "__fish_seen_subcommand_from perf" -a "analyze" -d "Analyze data"
complete -c nself -f -n "__fish_seen_subcommand_from perf" -a "benchmark" -d "Run benchmarks"
complete -c nself -f -n "__fish_seen_subcommand_from perf" -a "report" -d "Generate report"
complete -c nself -f -n "__fish_seen_subcommand_from perf" -a "compare" -d "Compare runs"

# bench subcommands
complete -c nself -f -n "__fish_seen_subcommand_from bench" -a "run" -d "Run benchmarks"
complete -c nself -f -n "__fish_seen_subcommand_from bench" -a "http" -d "HTTP load test"
complete -c nself -f -n "__fish_seen_subcommand_from bench" -a "db" -d "Database benchmark"
complete -c nself -f -n "__fish_seen_subcommand_from bench" -a "compare" -d "Compare results"
complete -c nself -f -n "__fish_seen_subcommand_from bench" -a "report" -d "Generate report"

# scale subcommands
complete -c nself -f -n "__fish_seen_subcommand_from scale" -a "up" -d "Scale up"
complete -c nself -f -n "__fish_seen_subcommand_from scale" -a "down" -d "Scale down"
complete -c nself -f -n "__fish_seen_subcommand_from scale" -a "auto" -d "Autoscaling"
complete -c nself -f -n "__fish_seen_subcommand_from scale" -a "status" -d "Scale status"
complete -c nself -f -n "__fish_seen_subcommand_from scale" -a "config" -d "Scale config"

# health subcommands
complete -c nself -f -n "__fish_seen_subcommand_from health" -a "check" -d "Run checks"
complete -c nself -f -n "__fish_seen_subcommand_from health" -a "dashboard" -d "Health dashboard"
complete -c nself -f -n "__fish_seen_subcommand_from health" -a "alert" -d "Configure alerts"
complete -c nself -f -n "__fish_seen_subcommand_from health" -a "history" -d "Health history"

# tenant subcommands
complete -c nself -f -n "__fish_seen_subcommand_from tenant" -a "init" -d "Initialize multi-tenancy"
complete -c nself -f -n "__fish_seen_subcommand_from tenant" -a "create" -d "Create tenant"
complete -c nself -f -n "__fish_seen_subcommand_from tenant" -a "list" -d "List tenants"
complete -c nself -f -n "__fish_seen_subcommand_from tenant" -a "show" -d "Show tenant details"
complete -c nself -f -n "__fish_seen_subcommand_from tenant" -a "delete" -d "Delete tenant"
complete -c nself -f -n "__fish_seen_subcommand_from tenant" -a "switch" -d "Switch tenant"
complete -c nself -f -n "__fish_seen_subcommand_from tenant" -a "suspend" -d "Suspend tenant"
complete -c nself -f -n "__fish_seen_subcommand_from tenant" -a "activate" -d "Activate tenant"
complete -c nself -f -n "__fish_seen_subcommand_from tenant" -a "billing" -d "Billing management"
complete -c nself -f -n "__fish_seen_subcommand_from tenant" -a "branding" -d "Branding customization"
complete -c nself -f -n "__fish_seen_subcommand_from tenant" -a "domains" -d "Domain management"
complete -c nself -f -n "__fish_seen_subcommand_from tenant" -a "email" -d "Email templates"
complete -c nself -f -n "__fish_seen_subcommand_from tenant" -a "themes" -d "Theme management"

# tenant billing subcommands
complete -c nself -f -n "__fish_seen_subcommand_from billing; and __fish_seen_subcommand_from tenant" -a "usage" -d "Usage tracking"
complete -c nself -f -n "__fish_seen_subcommand_from billing; and __fish_seen_subcommand_from tenant" -a "invoices" -d "Invoice management"
complete -c nself -f -n "__fish_seen_subcommand_from billing; and __fish_seen_subcommand_from tenant" -a "quotas" -d "Quota management"
complete -c nself -f -n "__fish_seen_subcommand_from billing; and __fish_seen_subcommand_from tenant" -a "plans" -d "Plan management"
complete -c nself -f -n "__fish_seen_subcommand_from billing; and __fish_seen_subcommand_from tenant" -a "upgrade" -d "Upgrade plan"
complete -c nself -f -n "__fish_seen_subcommand_from billing; and __fish_seen_subcommand_from tenant" -a "downgrade" -d "Downgrade plan"

# tenant branding subcommands
complete -c nself -f -n "__fish_seen_subcommand_from branding; and __fish_seen_subcommand_from tenant" -a "logo" -d "Logo management"
complete -c nself -f -n "__fish_seen_subcommand_from branding; and __fish_seen_subcommand_from tenant" -a "colors" -d "Color customization"
complete -c nself -f -n "__fish_seen_subcommand_from branding; and __fish_seen_subcommand_from tenant" -a "themes" -d "Theme selection"
complete -c nself -f -n "__fish_seen_subcommand_from branding; and __fish_seen_subcommand_from tenant" -a "css" -d "Custom CSS"
complete -c nself -f -n "__fish_seen_subcommand_from branding; and __fish_seen_subcommand_from tenant" -a "preview" -d "Preview branding"

# tenant domains subcommands
complete -c nself -f -n "__fish_seen_subcommand_from domains; and __fish_seen_subcommand_from tenant" -a "add" -d "Add domain"
complete -c nself -f -n "__fish_seen_subcommand_from domains; and __fish_seen_subcommand_from tenant" -a "remove" -d "Remove domain"
complete -c nself -f -n "__fish_seen_subcommand_from domains; and __fish_seen_subcommand_from tenant" -a "list" -d "List domains"
complete -c nself -f -n "__fish_seen_subcommand_from domains; and __fish_seen_subcommand_from tenant" -a "verify" -d "Verify domain"
complete -c nself -f -n "__fish_seen_subcommand_from domains; and __fish_seen_subcommand_from tenant" -a "ssl" -d "SSL management"
complete -c nself -f -n "__fish_seen_subcommand_from domains; and __fish_seen_subcommand_from tenant" -a "status" -d "Domain status"

# tenant themes subcommands
complete -c nself -f -n "__fish_seen_subcommand_from themes; and __fish_seen_subcommand_from tenant" -a "create" -d "Create theme"
complete -c nself -f -n "__fish_seen_subcommand_from themes; and __fish_seen_subcommand_from tenant" -a "edit" -d "Edit theme"
complete -c nself -f -n "__fish_seen_subcommand_from themes; and __fish_seen_subcommand_from tenant" -a "preview" -d "Preview theme"
complete -c nself -f -n "__fish_seen_subcommand_from themes; and __fish_seen_subcommand_from tenant" -a "activate" -d "Activate theme"
complete -c nself -f -n "__fish_seen_subcommand_from themes; and __fish_seen_subcommand_from tenant" -a "list" -d "List themes"

# monitor subcommands
complete -c nself -f -n "__fish_seen_subcommand_from monitor" -a "start" -d "Start monitoring"
complete -c nself -f -n "__fish_seen_subcommand_from monitor" -a "stop" -d "Stop monitoring"
complete -c nself -f -n "__fish_seen_subcommand_from monitor" -a "status" -d "Monitoring status"
complete -c nself -f -n "__fish_seen_subcommand_from monitor" -a "logs" -d "Monitoring logs"
complete -c nself -f -n "__fish_seen_subcommand_from monitor" -a "grafana" -d "Open Grafana"
complete -c nself -f -n "__fish_seen_subcommand_from monitor" -a "prometheus" -d "Open Prometheus"
complete -c nself -f -n "__fish_seen_subcommand_from monitor" -a "alertmanager" -d "Open Alertmanager"

# plugin subcommands
complete -c nself -f -n "__fish_seen_subcommand_from plugin" -a "list" -d "List plugins"
complete -c nself -f -n "__fish_seen_subcommand_from plugin" -a "install" -d "Install plugin"
complete -c nself -f -n "__fish_seen_subcommand_from plugin" -a "remove" -d "Remove plugin"
complete -c nself -f -n "__fish_seen_subcommand_from plugin" -a "update" -d "Update plugin"
complete -c nself -f -n "__fish_seen_subcommand_from plugin" -a "status" -d "Plugin status"

# frontend subcommands
complete -c nself -f -n "__fish_seen_subcommand_from frontend" -a "add" -d "Add frontend app"
complete -c nself -f -n "__fish_seen_subcommand_from frontend" -a "remove" -d "Remove frontend app"
complete -c nself -f -n "__fish_seen_subcommand_from frontend" -a "list" -d "List frontend apps"
complete -c nself -f -n "__fish_seen_subcommand_from frontend" -a "status" -d "Frontend status"
complete -c nself -f -n "__fish_seen_subcommand_from frontend" -a "config" -d "Configure frontend"

# history subcommands
complete -c nself -f -n "__fish_seen_subcommand_from history" -a "list" -d "List history"
complete -c nself -f -n "__fish_seen_subcommand_from history" -a "show" -d "Show details"
complete -c nself -f -n "__fish_seen_subcommand_from history" -a "stats" -d "Show statistics"
complete -c nself -f -n "__fish_seen_subcommand_from history" -a "clear" -d "Clear history"
complete -c nself -f -n "__fish_seen_subcommand_from history" -a "filter" -d "Filter entries"

# config subcommands
complete -c nself -f -n "__fish_seen_subcommand_from config" -a "show" -d "Show config"
complete -c nself -f -n "__fish_seen_subcommand_from config" -a "edit" -d "Edit config"
complete -c nself -f -n "__fish_seen_subcommand_from config" -a "validate" -d "Validate config"
complete -c nself -f -n "__fish_seen_subcommand_from config" -a "diff" -d "Compare configs"
complete -c nself -f -n "__fish_seen_subcommand_from config" -a "backup" -d "Backup config"
complete -c nself -f -n "__fish_seen_subcommand_from config" -a "restore" -d "Restore config"

# doctor options
complete -c nself -f -n "__fish_seen_subcommand_from doctor" -l fix -d "Auto-fix issues"
complete -c nself -f -n "__fish_seen_subcommand_from doctor" -l verbose -d "Verbose output"
complete -c nself -f -n "__fish_seen_subcommand_from doctor" -l help -d "Show help"

# Provider list for cloud server create
complete -c nself -f -n "__fish_seen_subcommand_from create" -a "digitalocean linode vultr hetzner ovh scaleway upcloud aws gcp azure oracle ibm contabo hostinger kamatera ssdnodes exoscale alibaba tencent yandex racknerd buyvm time4vps raspberrypi custom" -d "Provider"

# Environment completions
complete -c nself -f -n "__fish_seen_subcommand_from pull" -a "staging production" -d "Environment"
complete -c nself -f -n "__fish_seen_subcommand_from push" -a "staging" -d "Environment"
FISH_COMPLETION
}

# ============================================================================
# INSTALL HELPERS
# ============================================================================

install_bash() {
  local target="${1:-$HOME/.bashrc}"

  if [[ -f "$target" ]] && grep -q "nself completion bash" "$target" 2>/dev/null; then
    log_info "Bash completion already installed in $target"
    return 0
  fi

  echo '' >>"$target"
  echo '# nself shell completion' >>"$target"
  echo 'eval "$(nself completion bash)"' >>"$target"

  log_success "Bash completion installed in $target"
  log_info "Restart your shell or run: source $target"
}

install_zsh() {
  local target="${1:-$HOME/.zshrc}"

  if [[ -f "$target" ]] && grep -q "nself completion zsh" "$target" 2>/dev/null; then
    log_info "Zsh completion already installed in $target"
    return 0
  fi

  echo '' >>"$target"
  echo '# nself shell completion' >>"$target"
  echo 'eval "$(nself completion zsh)"' >>"$target"

  log_success "Zsh completion installed in $target"
  log_info "Restart your shell or run: source $target"
}

install_fish() {
  local target="${1:-$HOME/.config/fish/completions/nself.fish}"

  mkdir -p "$(dirname "$target")"
  generate_fish >"$target"

  log_success "Fish completion installed in $target"
  log_info "Completions will be available in new fish sessions"
}

# ============================================================================
# HELP
# ============================================================================

show_help() {
  cat <<'EOF'
nself completion - Generate shell completions

USAGE:
  nself completion <shell> [options]
  nself completion install <shell>

SHELLS:
  bash        Generate bash completion script
  zsh         Generate zsh completion script
  fish        Generate fish completion script

OPTIONS:
  install     Install completion to shell config file

EXAMPLES:
  # Output completion script (manual setup)
  nself completion bash >> ~/.bashrc
  nself completion zsh >> ~/.zshrc
  nself completion fish > ~/.config/fish/completions/nself.fish

  # Or use eval (add to shell config)
  eval "$(nself completion bash)"
  eval "$(nself completion zsh)"

  # Auto-install to config file
  nself completion install bash
  nself completion install zsh
  nself completion install fish

AFTER INSTALLATION:
  Restart your shell or source your config file:
    source ~/.bashrc   # bash
    source ~/.zshrc    # zsh

COMPLETIONS INCLUDE:
  - All top-level commands
  - Nested subcommands (2-3 levels deep)
  - Context-aware service/provider names
  - Deprecated commands (with warnings)

NOTES:
  - 'nself cloud' is deprecated, use 'nself provider' instead
  - Tab completion will work for all command levels
  - Dynamic completions for services from docker-compose.yml

EOF
}

# ============================================================================
# MAIN
# ============================================================================

main() {
  local command="${1:-help}"
  shift || true

  case "$command" in
    bash)
      generate_bash
      ;;
    zsh)
      generate_zsh
      ;;
    fish)
      generate_fish
      ;;
    install)
      local shell="${1:-bash}"
      case "$shell" in
        bash) install_bash ;;
        zsh) install_zsh ;;
        fish) install_fish ;;
        *)
          log_error "Unknown shell: $shell"
          return 1
          ;;
      esac
      ;;
    help | --help | -h)
      show_help
      ;;
    *)
      log_error "Unknown shell: $command"
      echo ""
      show_help
      return 1
      ;;
  esac
}

main "$@"
