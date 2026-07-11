# Testing coredump_exporter Manually

This document describes how to generate test coredumps on a Fedora VM and verify exporter output.

## Install Dependencies

```sh
sudo dnf install gcc golang systemd-devel systemd-udev
```

There is no separate `systemd-coredump` package on Fedora-family systems. The `coredumpctl` tool is provided by the systemd packaging, commonly the `systemd-udev` subpackage.

If the package name differs on your target distribution, query the provider with:

```sh
dnf provides '*/coredumpctl'
```

Build the exporter:

```sh
go build ./cmd/coredump_exporter
```

Run it in one terminal:

```sh
./coredump_exporter --web.listen-address=:9113
```

If running as a non-root user, make sure the user can read the journal. For a service user, membership in the `systemd-journal` group is usually required.

## Check coredumpctl

```sh
coredumpctl list
```

Enable core dumps in the current shell:

```sh
ulimit -c unlimited
```

## Generate SIGSEGV

Create `crash-segv.c`:

```c
int main(void) {
    int *p = 0;
    *p = 1;
    return 0;
}
```

Build and run:

```sh
gcc -g -o crash-segv crash-segv.c
./crash-segv
```

## Generate SIGFPE

Create `crash-fpe.c`:

```c
int main(void) {
    volatile int x = 1;
    volatile int y = 0;
    return x / y;
}
```

Build and run:

```sh
gcc -g -o crash-fpe crash-fpe.c
./crash-fpe
```

## Generate SIGABRT

Create `crash-abrt.c`:

```c
#include <stdlib.h>

int main(void) {
    abort();
}
```

Build and run:

```sh
gcc -g -o crash-abrt crash-abrt.c
./crash-abrt
```

## Verify Coredumps

Check that systemd recorded the crashes:

```sh
coredumpctl list
coredumpctl info ./crash-segv
```

Check exporter metrics:

```sh
curl -s http://localhost:9113/metrics | grep '^coredump_'
```

Expected output should include metrics similar to:

```text
coredump_count{boot_id="..."} 3
coredump_count_by_executable_signal{boot_id="...",exe="/path/to/crash-segv",signal="SIGSEGV"} 1
coredump_count_by_executable_signal{boot_id="...",exe="/path/to/crash-fpe",signal="SIGFPE"} 1
coredump_count_by_executable_signal{boot_id="...",exe="/path/to/crash-abrt",signal="SIGABRT"} 1
```

The executable paths depend on the directory where the test programs were built and executed.

## Troubleshooting

If no coredumps appear, check the core pattern and journal entries:

```sh
cat /proc/sys/kernel/core_pattern
journalctl -b -t systemd-coredump --no-pager
```

If the exporter cannot read the journal, run it as root for a quick test or grant the service user journal access through the `systemd-journal` group.
