# coredump_exporter

Prometheus exporter for systemd-coredump events recorded in the systemd journal.

The exporter reports coredumps from the current boot and groups valid coredump entries by executable path and signal name.

## Building

```sh
go build ./cmd/coredump_exporter
```

The exporter uses the systemd journal API through `github.com/coreos/go-systemd/v22/sdjournal`, so builds require cgo and systemd development headers.

On Fedora, install the build dependencies with:

```sh
sudo dnf install golang systemd-devel
```

For manual testing, `coredumpctl` is also useful. On Fedora-family systems it is provided by the systemd packaging, commonly the `systemd-udev` subpackage.

## Running

```sh
./coredump_exporter --web.listen-address=:9113
```

Metrics are exposed at:

```text
http://localhost:9113/metrics
```

The process needs permission to read the systemd journal. A dedicated service user can usually be granted access by adding it to the `systemd-journal` group.

A sample hardened systemd unit file is provided in [`coredump_exporter.service`](coredump_exporter.service). It runs the exporter as `coredump-exporter` with membership in the `systemd-journal` group.

## Metrics

| Metric | Type | Labels | Description |
| --- | --- | --- | --- |
| `coredump_count` | gauge | `boot_id` | Number of valid coredump journal entries for the current boot. |
| `coredump_count_by_executable_signal` | gauge | `boot_id`, `exe`, `signal` | Number of valid coredump journal entries grouped by executable and signal. |
| `coredump_malformed_entries` | gauge | `boot_id`, `reason` | Number of coredump-like journal entries skipped because required fields were missing or invalid. |
| `coredump_scrape_errors_total` | counter | none | Total number of failed journal scrapes. |

The exporter also exposes standard Go and process metrics from the Prometheus Go client.

## Malformed Entries

Coredump entries are excluded from `coredump_count` and reported through `coredump_malformed_entries` when:

- `COREDUMP_EXE` is missing: `reason="missing_exe"`
- `COREDUMP_EXE` is empty: `reason="empty_exe"`
- `COREDUMP_SIGNAL_NAME` is missing: `reason="missing_signal"`
- `COREDUMP_SIGNAL_NAME` is empty: `reason="empty_signal"`

## Example Output

```text
# HELP coredump_count Number of valid coredump journal entries for the current boot.
# TYPE coredump_count gauge
coredump_count{boot_id="d0b66454-0f6a-4e8e-ba42-ff27a1f3206b"} 3
# HELP coredump_count_by_executable_signal Number of valid coredump journal entries grouped by executable and signal for the current boot.
# TYPE coredump_count_by_executable_signal gauge
coredump_count_by_executable_signal{boot_id="d0b66454-0f6a-4e8e-ba42-ff27a1f3206b",exe="/usr/bin/example",signal="SIGSEGV"} 2
coredump_count_by_executable_signal{boot_id="d0b66454-0f6a-4e8e-ba42-ff27a1f3206b",exe="/usr/bin/example",signal="SIGABRT"} 1
```

## Prometheus Configuration

```yaml
scrape_configs:
  - job_name: coredump
    static_configs:
      - targets:
          - localhost:9113
```

## Notes

The exporter reads only entries from the current boot. It reads the boot ID from `/proc/sys/kernel/random/boot_id` and matches it against journald's `_BOOT_ID` field. Counts reset on reboot and may decrease if journal retention removes matching entries.

Journal entries with `PRIORITY` greater than 4 (notice, info, or debug) are excluded. This matches the warning-level filter used by the Python exporter and limits scraping to coredump messages at warning severity or higher.

The `exe` label contains the executable path recorded by systemd-coredump. This is useful for identifying crashing programs, but it can create high-cardinality metrics on systems with many distinct executable paths.

### Migration from the Python coredump-exporter

The Python based [coredump-exporter](https://gitlab.com/Ma27/coredump-exporter) uses different metric names:

| Python | Go |
| --- | --- |
| `coredumps_total{boot_id}` | `coredump_count{boot_id}` |
| `coredumps{boot_id,exe,signal_name}` | `coredump_count_by_executable_signal{boot_id,exe,signal}` |
