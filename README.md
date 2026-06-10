
mongodb_ftdc_viewer is a reimagined and performance-focused evolution of zelmario/Big-hole - kudos to zelmario for the original work.

It provides a fast, reliable way to process MongoDB diagnostics data and push it to **VictoriaMetrics** at high speed. Metrics can then be explored through a Docker-hosted Grafana instance.

## Performance

The FTDC exporter is built with a parallel pipeline engine that achieves high throughput on modern hardware:

- **Parallel file processing**: Multiple FTDC files are decoded concurrently (configurable via `--parallel`, default 10).
- **Writer pool**: A pool of HTTP connections to VictoriaMetrics allows concurrent writes without serialization bottlenecks.
- **Batch-oriented pipeline**: FTDC metrics are streamed in configurable batches, sorted and serialized once per batch instead of per point.
- **Zero-alloc hot paths**: String escaping, metric normalization, and line-protocol encoding use pooled builders and pre-allocated replace tables to minimize GC pressure.
- **Single-pass include-file**: The metric filter list is parsed once at startup rather than re-read for every file.

Benchmark (34 FTDC files, ~950K metrics, batch size 200, parallel 10):

| Optimization stage | Wall time | Improvement |
|---|---|---|
| Baseline (per-file writers, per-point sorting) | ~5 min | — |
| + Shared writer + pre-sorted fields + pooled allocs | ~3 min | −40% |
| + Writer pool + single-pass include-file | **~1.5 min** | **−70% total** |

![Screenshoot](https://github.com/devops-land/mongodb_ftdc_viewer/blob/main/ftdc_exporter.png?raw=true)

## Prerequisites
- Docker and Docker-compose

## Installation
1. Clone the repository `git clone https://github.com/devops-land/mongodb_ftdc_viewer.git`
2. Navigate to the project directory `cd mongodb_ftdc_viewer`
3. Make the main script executable: `chmod +x run.sh`
4. Build the docker images `docker-compose build`

## Usage
1. Run the script `./run.sh --input-dir <DIAGNOSTICS_DATA_DIRECTORY>`

The script accepts several optional flags to tune performance:

| Flag | Default | Description |
|------|---------|-------------|
| `--parallel` | 10 | Number of FTDC files to process concurrently. Increase for more CPU cores. |
| `--batch-size` | 200 | FTDC documents per batch. Larger batches reduce HTTP overhead but increase memory per batch. |
| `--bucket` | bucket | Logical namespace tag attached to every metric. |
| `--victoria-retention-days` | 30 | How long VictoriaMetrics retains data. |
| `--victoria-data-directory` | /tmp/victoria_data | Host path for VictoriaMetrics persistent storage. |

***Note: you need to do those steps every time you need to read new diagnostic data files***

The script will decode all the diagnostic data files (time depends on file count, metric volume, and hardware) and launch Docker containers:
- **ftdc_exporter**: Parses FTDC files and sends metrics to VictoriaMetrics
- **victoriametrics**: Time-series database storing metrics
- **grafana**: Visualization layer with pre-built dashboards


![Screenshoot](https://github.com/devops-land/mongodb_ftdc_viewer/blob/main/dashboard.png?raw=true)


## Read Other FTDC Data:
To read other FTDC data you need to stop the containers with `Ctrl-C`, delete the `diagnostic.data` directory files and copy the new ones.


## How to get more metrics
There is a file named `metrics_to_get.txt` that contains the list of metrics to retrieve. If you want to gather more metrics, simply add the name of the desired metric to this file.
You'll find a complete list of all available metrics in another file called `metrics.txt`. Just add the metric you want to retrieve to `metrics_to_get.txt`, and the script will collect it.

You can use VictoriaMetrics to view and query metrics:
```bash
http://localhost:8428/vmui
```
VictoriaMetrics UI provides Prometheus/MetricsQL query interface.

### Accessing Grafana

If you want to edit the dashboard, you can log in to grafana:
```bash
http://localhost:3001/
user: admin
pass: admin
```

Then, you can save it to `grafana/dashboard/dashboard.json`.

The default dashboard is available at:
```bash
http://localhost:3001/d/ddnw277huiv40ae/ftdc-dashboard
```

***Have in mind that after any modification on the metrics_to_get.txt file or the dashboard, you need to rebuild the container with `docker-compose build`***

## Architecture

### Storage Backend
By default, the tool uses **VictoriaMetrics** as the storage backend. VictoriaMetrics is a fast, cost-effective time-series database compatible with Prometheus query language (PromQL/MetricsQL).

### Metric Naming
Metrics ingested into VictoriaMetrics follow this naming pattern:
- Original: `serverStatus.mem.virtual`
- Stored as: `ftdc_serverStatus_mem_virtual{hostname="...", version="..."}`

All dots in FTDC metric names are replaced with underscores, and tags (hostname, version) are stored as Prometheus-style labels.

### Data Retention
By default, VictoriaMetrics retains data for 30 days. You can adjust this via the `VICTORIA_RETENTION_DAYS` environment variable in `run.sh`.


## License
This project is licensed under the MIT License - see the LICENSE file for details.

## Contributing
All Contributions are welocme!

