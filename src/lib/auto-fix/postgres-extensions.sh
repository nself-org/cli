#!/usr/bin/env bash


# Comprehensive Postgres extension validation and information
# Uses delimited lists for compatibility with Bash 3.2+

# Extension information as delimited string (name|description|category|safe)
POSTGRES_EXTENSIONS_INFO="

set -euo pipefail

uuid-ossp|UUID generation functions|Core|true
pgcrypto|Cryptographic functions|Core|true
citext|Case-insensitive text type|Core|true
hstore|Key-value store|Core|true
pg_trgm|Trigram text search|Core|true
btree_gin|GIN index support for common types|Core|true
btree_gist|GiST index support for common types|Core|true
postgres_fdw|Foreign data wrapper for PostgreSQL|Core|true
file_fdw|Foreign data wrapper for files|Core|true
pg_stat_statements|Query performance statistics|Core|true
tablefunc|Cross tabulation functions|Core|true
unaccent|Text search dictionary for unaccented matching|Core|true
intarray|Functions for 1-D arrays of integers|Core|true
ltree|Hierarchical tree-like structures|Core|true
xml2|XPath querying and XSLT|Core|true
fuzzystrmatch|Fuzzy string matching|Core|true
cube|Multi-dimensional cubes|Core|true
earthdistance|Great circle distance calculations|Core|true
isn|International product numbering standards|Core|true
lo|Large object maintenance|Core|true
pg_buffercache|Examine shared buffer cache|Core|true
pg_prewarm|Prewarm buffer cache|Core|true
pg_visibility|Visibility map examination|Core|true
pgrowlocks|Row locking information|Core|true
pgstattuple|Tuple-level statistics|Core|true
sslinfo|SSL certificate information|Core|true
tsm_system_rows|TABLESAMPLE method SYSTEM_ROWS|Core|true
tsm_system_time|TABLESAMPLE method SYSTEM_TIME|Core|true
adminpack|Administrative functions|Core|true
amcheck|Verify index integrity|Core|true
bloom|Bloom filter index|Core|true
dblink|Connect to other PostgreSQL databases|Core|true
dict_int|Dictionary for integers|Core|true
dict_xsyn|Dictionary of synonyms|Core|true
pageinspect|Inspect database pages|Core|true
pg_freespacemap|Free space map|Core|true
pg_surgery|Perform surgery on relation data|Core|true
pg_walinspect|Inspect WAL|Core|true
pgaudit|Session and object audit logging|Contrib|true
timescaledb|Time-series database|Third-party|true
postgis|Geographic information system|Third-party|true
postgis_topology|PostGIS topology support|Third-party|true
postgis_raster|PostGIS raster support|Third-party|true
postgis_tiger_geocoder|PostGIS TIGER geocoder|Third-party|true
address_standardizer|Address standardizer|Third-party|true
address_standardizer_data_us|US address data|Third-party|true
pgrouting|Routing functionality|Third-party|true
pgvector|Vector similarity search|Third-party|true
pg_cron|Job scheduler|Third-party|true
pg_partman|Partition management|Third-party|true
pg_repack|Online table reorganization|Third-party|true
pglogical|Logical replication|Third-party|true
wal2json|WAL to JSON output|Third-party|true
pg_jsonschema|JSON Schema validation|Third-party|true
pg_graphql|GraphQL support|Third-party|true
pg_net|HTTP client|Third-party|true
plv8|JavaScript language|Third-party|false
plpython3u|Python 3 language|Core|false
plperlu|Perl language (untrusted)|Core|false
pltclu|Tcl language (untrusted)|Core|false
plr|R language|Third-party|false
pljava|Java language|Third-party|false
plsh|Shell language|Third-party|false
multicorn|Foreign data wrapper framework|Third-party|false
citus|Distributed PostgreSQL|Third-party|false
age|Graph database|Third-party|false
orioledb|Table storage engine|Third-party|false
pg_lakehouse|Data lakehouse|Third-party|false
"

# Extension groups as delimited string (group_name|extensions)
EXTENSION_GROUPS="
spatial|postgis,postgis_topology,postgis_raster,address_standardizer
search|pg_trgm,unaccent,fuzzystrmatch
monitoring|pg_stat_statements,pg_buffercache,pgstattuple
crypto|pgcrypto,uuid-ossp
timeseries|timescaledb,pg_partman
ml|pgvector,plpython3u
replication|pglogical,wal2json
"

validate_postgres_extension() {
  local ext="$1"
  local ext_lower=$(echo "$ext" | tr '[:upper:]' '[:lower:]')

  # Check if extension exists in our database
  local ext_info=$(echo "$POSTGRES_EXTENSIONS_INFO" | grep "^$ext_lower|" | head -1)
  if [[ -n "$ext_info" ]]; then
    IFS='|' read -r name description category available <<<"$ext_info"

    if [[ "$available" == "false" ]]; then
      return 2 # Extension exists but not available in standard image
    fi
    return 0 # Valid and available
  fi

  # Check common misspellings
  case "$ext_lower" in
    "uuid_ossp" | "uuid-generate")
      echo "uuid-ossp" # Return correct name
      return 3         # Misspelled
      ;;
    "pg_crypto" | "pg-crypto")
      echo "pgcrypto"
      return 3
      ;;
    "time_scale" | "time-scale" | "timescale")
      echo "timescaledb"
      return 3
      ;;
    "post_gis" | "post-gis")
      echo "postgis"
      return 3
      ;;
    "pg_vector" | "pg-vector")
      echo "pgvector"
      return 3
      ;;
    *)
      return 1 # Unknown extension
      ;;
  esac
}

get_extension_info() {
  local ext="$1"
  local ext_lower=$(echo "$ext" | tr '[:upper:]' '[:lower:]')

  local ext_info=$(echo "$POSTGRES_EXTENSIONS_INFO" | grep "^$ext_lower|" | head -1)
  if [[ -n "$ext_info" ]]; then
    IFS='|' read -r name description category available <<<"$ext_info"
    echo "$description"
  else
    echo "Unknown extension"
  fi
}

suggest_extension_alternatives() {
  local ext="$1"
  local purpose=""

  # Try to determine the purpose based on the extension name
  case "$ext" in
    *crypt* | *security* | *hash*)
      purpose="crypto"
      echo "Consider using: pgcrypto"
      ;;
    *uuid* | *guid*)
      purpose="uuid"
      echo "Consider using: uuid-ossp"
      ;;
    *time* | *series*)
      purpose="timeseries"
      echo "Consider using: timescaledb"
      ;;
    *geo* | *spatial* | *map*)
      purpose="spatial"
      echo "Consider using: postgis"
      ;;
    *vector* | *embed* | *ml*)
      purpose="ml"
      echo "Consider using: pgvector"
      ;;
    *search* | *text*)
      purpose="search"
      echo "Consider using: pg_trgm, fuzzystrmatch"
      ;;
    *json*)
      purpose="json"
      echo "Consider using: built-in JSON/JSONB types"
      ;;
    *)
      echo "Check available extensions with: SELECT * FROM pg_available_extensions;"
      ;;
  esac
}

check_extension_compatibility() {
  local extensions="$1"
  local warnings=()

  IFS=',' read -r -a ext_array <<<"$extensions"

  # Check for conflicting extensions
  # shellcheck disable=SC2199 # Intentional array concatenation in regex match
  if [[ " ${ext_array[*]} " =~ " citus " ]] && [[ " ${ext_array[*]} " =~ " timescaledb " ]]; then
    warnings+=("Citus and TimescaleDB may conflict - use one or the other")
  fi

  # Check for heavy extensions
  local heavy_count=0
  for ext in "${ext_array[@]}"; do
    case "$ext" in
      timescaledb | postgis | citus | orioledb)
        ((heavy_count++))
        ;;
    esac
  done

  if [[ $heavy_count -gt 2 ]]; then
    warnings+=("Multiple heavy extensions may impact performance")
  fi

  # Check for deprecated extensions
  for ext in "${ext_array[@]}"; do
    case "$ext" in
      xml2)
        warnings+=("xml2 is deprecated - use built-in XML functions")
        ;;
      tsearch2)
        warnings+=("tsearch2 is obsolete - use built-in full text search")
        ;;
    esac
  done

  # Return warnings
  if [[ ${#warnings[@]} -gt 0 ]]; then
    printf '%s\n' "${warnings[@]}"
    return 1
  fi

  return 0
}

recommend_extensions() {
  local use_case="$1"

  case "$use_case" in
    "api" | "backend")
      echo "uuid-ossp,pgcrypto,pg_trgm"
      ;;
    "analytics" | "reporting")
      echo "timescaledb,pg_stat_statements,tablefunc"
      ;;
    "geospatial" | "maps")
      echo "postgis,postgis_topology,pgrouting"
      ;;
    "ml" | "ai" | "embeddings")
      echo "pgvector,plpython3u"
      ;;
    "search" | "fulltext")
      echo "pg_trgm,unaccent,fuzzystrmatch"
      ;;
    "audit" | "compliance")
      echo "pgaudit,pg_stat_statements"
      ;;
    *)
      echo "uuid-ossp,pgcrypto"
      ;;
  esac
}

generate_extension_sql() {
  local extensions="$1"
  local output=""

  IFS=',' read -r -a ext_array <<<"$extensions"

  for ext in "${ext_array[@]}"; do
    ext=$(echo "$ext" | tr -d ' ')
    printf "CREATE EXTENSION IF NOT EXISTS \"%s\";\n" "$ext"
  done
}

export -f validate_postgres_extension
export -f get_extension_info
export -f suggest_extension_alternatives
export -f check_extension_compatibility
export -f recommend_extensions
export -f generate_extension_sql
