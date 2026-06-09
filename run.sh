#!/bin/bash


# default configuration
export PARALLEL=${PARALLEL:-10}
export BATCH_SIZE=${BATCH_SIZE:-200}
export STORAGE_BACKEND=${STORAGE_BACKEND:-victoriametrics}
export VICTORIA_DATA_DIRECTORY=${VICTORIA_DATA_DIRECTORY:-/tmp/victoria_data}
export VICTORIA_RETENTION_DAYS=${VICTORIA_RETENTION_DAYS:-30}
export VICTORIA_TOKEN=${VICTORIA_TOKEN:-}
export VICTORIA_TENANT=${VICTORIA_TENANT:-}
export VICTORIA_USERNAME=${VICTORIA_USERNAME:-}
export VICTORIA_PASSWORD=${VICTORIA_PASSWORD:-}
export GRAFANA_ADMIN_PASSWORD=${GRAFANA_ADMIN_PASSWORD:-"$(uuidgen)"}
export INFLUX_BUCKET=${INFLUX_BUCKET:-bucket}

# --- Parse CLI args ---
while [[ "$#" -gt 0 ]]; do
  case "$1" in
    --input-dir) export INPUT_DIR="$2"; shift 2 ;;
    --influx-bucket) export INFLUX_BUCKET="$2"; shift 2 ;;
    --parallel) export PARALLEL="$2"; shift 2 ;;
    --batch-size) export BATCH_SIZE="$2"; shift 2 ;;
    --victoria-data-directory) export VICTORIA_DATA_DIRECTORY="$2"; shift 2 ;;
    --victoria-retention-days) export VICTORIA_RETENTION_DAYS="$2"; shift 2 ;;
    -h|--help)
      cat <<'EOF'
Usage: ./run.sh --input-dir /path/to/diagnostic.data [options]

Common options:
  --parallel <int>               Number of files to process in parallel (default: 10)
  --batch-size <int>             FTDC docs per batch (default: 200)
  --influx-bucket <name>         Logical bucket/tag namespace (used for both backends)
  --victoria-data-directory <p>  Host path to persist VictoriaMetrics data (default: /tmp/victoria_data)
  --victoria-retention-days <n>  Data retention period (default: 30)

VictoriaMetrics backend (default):
  STORAGE_BACKEND=victoriametrics (default)
  --victoria-url <url>           Override VictoriaMetrics base URL (default: http://victoriametrics:8428)
  --victoria-token <token>       Optional token header for /api/v2/write
  --victoria-tenant <tenant>     Optional multi-tenant header (X-Scope-OrgID)
  --victoria-username <user>     Optional basic auth user
  --victoria-password <pass>     Optional basic auth password

Example:
  ./run.sh --input-dir ./diagnostic.data --victoria-data-directory /data/vm \
           --victoria-retention-days 14 --parallel 6

To use InfluxDB instead, export STORAGE_BACKEND=influx and pass the legacy --influx-* flags.
EOF
      exit 0
      ;;
    *)
      echo "Unknown option: $1"
      exit 1
      ;;
  esac
done



if [ -z "$INPUT_DIR" ]; then
  echo "Error: --input-dir not specified."
  exit 1
fi


if [ -z "$BATCH_SIZE" ]; then
  echo "Error: No batch size specified."
  echo "Example: --batch-size 100"
  exit 1
fi


# List contents of the directory
echo "Contents of '$INPUT_DIR':"
ls -la "$INPUT_DIR"

# Ask for user confirmation
read -p "Do you want to proceed with this directory? (y/n): " answer

case "$answer" in
  y|Y )
    echo "Proceeding..."
    ;;
  * )
    echo "Aborting."
    exit 1
    ;;
esac



# Function to handle Ctrl-C (SIGINT)
cleanup() {
    echo "Stopping Docker containers..."
    $ENVS docker-compose down -v
    echo "Cleaning VictoriaMetrics data directory: $VICTORIA_DATA_DIRECTORY"
    rm -rf "$VICTORIA_DATA_DIRECTORY"
    echo "Docker containers stopped."
    exit 0
}

# Trap Ctrl-C (SIGINT) to run the cleanup function
trap cleanup SIGINT


echo "Checking for image changes and building if needed..."
docker-compose build

# remove any running container
docker-compose down -v

# Start Docker containers in detached mode
docker-compose up -d

echo "🔐 Credentials:"
echo "--------------------------"
echo "VictoriaMetrics URL      = http://localhost:8428/"
echo "Grafana Dashboard URL    = http://localhost:3001/d/ddnw277huiv40ae/ftdc-dashboard"
echo "Grafana user             = admin"
echo "Grafana Password         = $GRAFANA_ADMIN_PASSWORD"


# Stream exporter logs and auto-detect the exact Grafana URL it prints (includes from/to)
FTDC_EXPORTER_CID=$(docker-compose ps -q ftdc_exporter)
if [ -z "$FTDC_EXPORTER_CID" ]; then
  echo "Could not resolve ftdc_exporter container id."
  exit 1
fi

echo "\nFollowing ftdc_exporter logs (Ctrl+C to stop and cleanup)…\n"

# Print the first exporter-generated Grafana URL we see, then continue streaming logs.
docker logs -f "$FTDC_EXPORTER_CID" 2>&1 | while IFS= read -r line; do
  echo "$line"
  if [[ -z "${__PRINTED_GRAFANA_URL}" && "$line" =~ (http://localhost:3001/d/[^[:space:]]+) ]]; then
    __PRINTED_GRAFANA_URL=1
    echo ""
    echo "Grafana Dashboard URL (auto-detected): ${BASH_REMATCH[1]}"
    echo ""
  fi
done
