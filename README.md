# Polygon Validator Monitor
Polygon Validator Monitor is a tool for Polygon validators or stakers to monitor the performance of the Polygon validator set or of a particular validator. The tool exposes several [Prometheus](https://prometheus.io/) metrics, which can be used in conjunction with other tools such as [Grafana](https://grafana.com/) and [AlertManager](https://prometheus.io/docs/alerting/latest/alertmanager/) to view the data over time or alert based on several conditions.


## Installation Guide

### Requirements:
- An ETH RPC, preferably with historical data if running for the first time, and you want to get checkpoints included in blocks before the last 128.
- List of validators' signer keys to monitor.

### Setup
1. Install `go` v1.20+ (we are using go1.20.3) and `make` (part of `build-essential`).
3. In `config/config.json`:
    1. Update `"ETHRpcUrl"` with your own ETH node.
    2. Update `"PrometheusPort"` to your preferred port for the metrics.
    3. Update `"DatabaseLocation"` to the path where the database should be stored.
    4. Update `"PublicKeys"` with a list of the validators' signer keys to monitor. You can set this to `["*"]`, which will monitor all validators.
    5. Update `"ContinueFromBlock"` to the ETH block number the tool should start looking for checkpoints from. If you are running a non-archival ETH node with default pruning, you might encounter issues if you try setting this to anything more than `(current block height - 128)`.
3. Build the tool with `make build`. This will generate the binary in `build/bin`.
4. Run the tool and specify the path to the config with the flag `--config=/path/to/you/config/file`. By default, the tool will look for it in `config/config.json`, but this will not work if your working directory is different. If running the tool on Linux, you can use the provided service file (`setup/polygon-monitor.service`).

### Usage
After running the binary, Prometheus metrics are exported on localhost on your chosen port. The tool queries the provided ETH RPC every minute for any new checkpoint events included in each ETH block. In case of a new checkpoint, it processes it and updates all corresponding metrics. The data is also saved to an sqlite3 database specified in the config (by default in `data/checkpoint_data.db`).

### Updating
Before updating to a newer version or commit, we always recommend saving a copy of your database (i.e. `data/checkpoint_data.db`), so that you can rollback.

## Metrics
The tool contains the following list of Prometheus metrics:
1. `current_checkpoint -> int`: The last checkpoint processed by the tool.
2. `current_block_number -> int`: The last ETH block number processed by the tool.
3. `checkpoints_signed{validator, range} -> int`: The number of checkpoints signed by a validator for the given range {700 checkpoints, total}.
4. `checkpoints_total{range} -> int`: The number of checkpoints in a given range {700 checkpoints, total}.
5. `validator_performance{validator, range} -> float`: The performance of a validator for the given range {700 checkpoints, total}. It is the number of checkpoints signed by the validator for a certain range, divided by the total number of checkpoints in said range.
6. `current_performance_benchmark -> float`: The current performance benchmark of the Polygon validator set. If a validator's performance falls below this value, they enter the grace period.
7. `checkpoints_to_performance_benchmark{validator} -> int`: The number of checkpoints the validator must miss in order to enter the grace period, based on the current performance benchmark.
8. `checkpoints_to_reduction{validator} -> int`: How many checkpoints a validator has to go through before seeing an improvement in their performance of the last 700 checkpoints.

For any metric that contains a `validator` label, the validator must be monitored (i.e. included in `"PublicKeys"` in the config) in order for it to be included in the mentioned metrics.