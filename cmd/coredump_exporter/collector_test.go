package main

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
	dto "github.com/prometheus/client_model/go"
)

const testBootID = "b17c4ab7-9706-4da4-9871-19c130c8701e"

type fakeJournalReader struct {
	entries []journalEntry
	err     error
}

func (r fakeJournalReader) Entries(string) ([]journalEntry, error) {
	return r.entries, r.err
}

func fixedBootID() (string, error) {
	return testBootID, nil
}

func TestCollectorReportsEmptyJournal(t *testing.T) {
	t.Parallel()

	collector := newCoredumpCollector(fakeJournalReader{}, fixedBootID)

	expected := `
# HELP coredump_count Number of valid coredump journal entries for the current boot.
# TYPE coredump_count gauge
coredump_count{boot_id="b17c4ab7-9706-4da4-9871-19c130c8701e"} 0
# HELP coredump_malformed_entries Number of coredump-like journal entries skipped because required fields were missing or invalid.
# TYPE coredump_malformed_entries gauge
coredump_malformed_entries{boot_id="b17c4ab7-9706-4da4-9871-19c130c8701e",reason="empty_exe"} 0
coredump_malformed_entries{boot_id="b17c4ab7-9706-4da4-9871-19c130c8701e",reason="empty_signal"} 0
coredump_malformed_entries{boot_id="b17c4ab7-9706-4da4-9871-19c130c8701e",reason="missing_exe"} 0
coredump_malformed_entries{boot_id="b17c4ab7-9706-4da4-9871-19c130c8701e",reason="missing_signal"} 0
# HELP coredump_scrape_errors_total Total number of failed journal scrapes.
# TYPE coredump_scrape_errors_total counter
coredump_scrape_errors_total 0
`

	if err := testutil.CollectAndCompare(collector, strings.NewReader(expected)); err != nil {
		t.Fatal(err)
	}
}

func TestCollectorGroupsValidCoredumps(t *testing.T) {
	t.Parallel()

	collector := newCoredumpCollector(fakeJournalReader{entries: []journalEntry{
		{fieldCoredumpExe: "/usr/bin/foo", fieldCoredumpSignal: "SIGSEGV"},
		{fieldCoredumpExe: "/usr/bin/foo", fieldCoredumpSignal: "SIGSEGV"},
		{fieldCoredumpExe: "/usr/bin/foo", fieldCoredumpSignal: "SIGABRT"},
		{fieldCoredumpExe: "/usr/bin/bar", fieldCoredumpSignal: "SIGSEGV"},
	}}, fixedBootID)

	expected := `
# HELP coredump_count Number of valid coredump journal entries for the current boot.
# TYPE coredump_count gauge
coredump_count{boot_id="b17c4ab7-9706-4da4-9871-19c130c8701e"} 4
# HELP coredump_count_by_executable_signal Number of valid coredump journal entries grouped by executable and signal for the current boot.
# TYPE coredump_count_by_executable_signal gauge
coredump_count_by_executable_signal{boot_id="b17c4ab7-9706-4da4-9871-19c130c8701e",exe="/usr/bin/bar",signal="SIGSEGV"} 1
coredump_count_by_executable_signal{boot_id="b17c4ab7-9706-4da4-9871-19c130c8701e",exe="/usr/bin/foo",signal="SIGABRT"} 1
coredump_count_by_executable_signal{boot_id="b17c4ab7-9706-4da4-9871-19c130c8701e",exe="/usr/bin/foo",signal="SIGSEGV"} 2
# HELP coredump_malformed_entries Number of coredump-like journal entries skipped because required fields were missing or invalid.
# TYPE coredump_malformed_entries gauge
coredump_malformed_entries{boot_id="b17c4ab7-9706-4da4-9871-19c130c8701e",reason="empty_exe"} 0
coredump_malformed_entries{boot_id="b17c4ab7-9706-4da4-9871-19c130c8701e",reason="empty_signal"} 0
coredump_malformed_entries{boot_id="b17c4ab7-9706-4da4-9871-19c130c8701e",reason="missing_exe"} 0
coredump_malformed_entries{boot_id="b17c4ab7-9706-4da4-9871-19c130c8701e",reason="missing_signal"} 0
# HELP coredump_scrape_errors_total Total number of failed journal scrapes.
# TYPE coredump_scrape_errors_total counter
coredump_scrape_errors_total 0
`

	if err := testutil.CollectAndCompare(collector, strings.NewReader(expected)); err != nil {
		t.Fatal(err)
	}
}

func TestCollectorReportsMalformedEntries(t *testing.T) {
	t.Parallel()

	collector := newCoredumpCollector(fakeJournalReader{entries: []journalEntry{
		{fieldCoredumpSignal: "SIGSEGV"},
		{fieldCoredumpExe: "/usr/bin/foo"},
		{fieldCoredumpExe: "", fieldCoredumpSignal: "SIGSEGV"},
		{fieldCoredumpExe: "   ", fieldCoredumpSignal: "SIGSEGV"},
		{fieldCoredumpExe: "/usr/bin/bar", fieldCoredumpSignal: ""},
		{fieldCoredumpExe: "/usr/bin/baz", fieldCoredumpSignal: "   "},
		{fieldCoredumpExe: "/usr/bin/qux", fieldCoredumpSignal: "SIGFPE"},
	}}, fixedBootID)

	expected := `
# HELP coredump_count Number of valid coredump journal entries for the current boot.
# TYPE coredump_count gauge
coredump_count{boot_id="b17c4ab7-9706-4da4-9871-19c130c8701e"} 1
# HELP coredump_count_by_executable_signal Number of valid coredump journal entries grouped by executable and signal for the current boot.
# TYPE coredump_count_by_executable_signal gauge
coredump_count_by_executable_signal{boot_id="b17c4ab7-9706-4da4-9871-19c130c8701e",exe="/usr/bin/qux",signal="SIGFPE"} 1
# HELP coredump_malformed_entries Number of coredump-like journal entries skipped because required fields were missing or invalid.
# TYPE coredump_malformed_entries gauge
coredump_malformed_entries{boot_id="b17c4ab7-9706-4da4-9871-19c130c8701e",reason="empty_exe"} 2
coredump_malformed_entries{boot_id="b17c4ab7-9706-4da4-9871-19c130c8701e",reason="empty_signal"} 2
coredump_malformed_entries{boot_id="b17c4ab7-9706-4da4-9871-19c130c8701e",reason="missing_exe"} 1
coredump_malformed_entries{boot_id="b17c4ab7-9706-4da4-9871-19c130c8701e",reason="missing_signal"} 1
# HELP coredump_scrape_errors_total Total number of failed journal scrapes.
# TYPE coredump_scrape_errors_total counter
coredump_scrape_errors_total 0
`

	if err := testutil.CollectAndCompare(collector, strings.NewReader(expected)); err != nil {
		t.Fatal(err)
	}
}

func failingBootID() (string, error) {
	return "", errors.New("boot ID unavailable")
}

func TestCollectorReturnsInvalidMetricOnBootIDError(t *testing.T) {
	t.Parallel()

	collector := newCoredumpCollector(fakeJournalReader{}, failingBootID)

	metricCh := make(chan prometheus.Metric)
	go func() {
		collector.Collect(metricCh)
		close(metricCh)
	}()

	var invalidFound bool
	var scrapeErrorsFound bool
	for metric := range metricCh {
		pb := &dto.Metric{}
		if err := metric.Write(pb); err != nil && strings.Contains(err.Error(), "boot ID unavailable") {
			invalidFound = true
		}
		if strings.Contains(metric.Desc().String(), "coredump_scrape_errors_total") {
			scrapeErrorsFound = true
		}
	}

	if !invalidFound {
		t.Fatal("expected invalid metric for boot ID error")
	}
	if !scrapeErrorsFound {
		t.Fatal("expected scrape error counter metric")
	}
}

func TestCollectorReturnsInvalidMetricOnJournalError(t *testing.T) {
	t.Parallel()

	collector := newCoredumpCollector(fakeJournalReader{err: errors.New("journal unavailable")}, fixedBootID)

	metricCh := make(chan prometheus.Metric)
	go func() {
		collector.Collect(metricCh)
		close(metricCh)
	}()

	var invalidFound bool
	var scrapeErrorsFound bool
	for metric := range metricCh {
		pb := &dto.Metric{}
		if err := metric.Write(pb); err != nil && strings.Contains(err.Error(), "journal unavailable") {
			invalidFound = true
		}
		if strings.Contains(metric.Desc().String(), "coredump_scrape_errors_total") {
			scrapeErrorsFound = true
		}
	}

	if !invalidFound {
		t.Fatal("expected invalid metric for journal error")
	}
	if !scrapeErrorsFound {
		t.Fatal("expected scrape error counter metric")
	}
}

func TestCollectorScrapeErrorCounterAccumulates(t *testing.T) {
	t.Parallel()

	collector := newCoredumpCollector(fakeJournalReader{err: errors.New("journal unavailable")}, fixedBootID)

	collectMetrics(t, collector)
	metrics := collectMetrics(t, collector)

	for _, metric := range metrics {
		if !strings.Contains(metric.Desc().String(), "coredump_scrape_errors_total") {
			continue
		}

		pb := &dto.Metric{}
		if err := metric.Write(pb); err != nil {
			t.Fatalf("write scrape error metric: %v", err)
		}
		if got, want := pb.GetCounter().GetValue(), 2.0; got != want {
			t.Fatalf("scrape errors = %v, want %v", got, want)
		}
		return
	}

	t.Fatal("expected scrape error counter metric")
}

func collectMetrics(t *testing.T, collector prometheus.Collector) []prometheus.Metric {
	t.Helper()

	metricCh := make(chan prometheus.Metric)
	go func() {
		collector.Collect(metricCh)
		close(metricCh)
	}()

	var metrics []prometheus.Metric
	for metric := range metricCh {
		metrics = append(metrics, metric)
	}
	return metrics
}

func TestReadBootID(t *testing.T) {
	t.Run("trims whitespace", func(t *testing.T) {
		setBootIDPath(t, writeBootIDFile(t, "  "+testBootID+"\n"))

		got, err := readBootID()
		if err != nil {
			t.Fatalf("readBootID() error = %v", err)
		}
		if got != testBootID {
			t.Fatalf("readBootID() = %q, want %q", got, testBootID)
		}
	})

	t.Run("rejects empty boot ID", func(t *testing.T) {
		setBootIDPath(t, writeBootIDFile(t, " \n\t"))

		_, err := readBootID()
		if err == nil || !strings.Contains(err.Error(), "empty boot ID") {
			t.Fatalf("readBootID() error = %v, want empty boot ID", err)
		}
	})

	t.Run("returns read error", func(t *testing.T) {
		setBootIDPath(t, filepath.Join(t.TempDir(), "missing-boot-id"))

		if _, err := readBootID(); err == nil {
			t.Fatal("readBootID() error = nil, want read error")
		}
	})
}

func setBootIDPath(t *testing.T, path string) {
	t.Helper()

	previous := bootIDPath
	bootIDPath = path
	t.Cleanup(func() {
		bootIDPath = previous
	})
}

func writeBootIDFile(t *testing.T, content string) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), "boot_id")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write boot ID file: %v", err)
	}
	return path
}

func TestPriorityFilter(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		fields map[string]string
		want   bool
	}{
		{name: "missing priority", fields: map[string]string{}, want: true},
		{name: "invalid priority", fields: map[string]string{fieldPriority: "invalid"}, want: true},
		{name: "emergency", fields: map[string]string{fieldPriority: "0"}, want: true},
		{name: "warning", fields: map[string]string{fieldPriority: "4"}, want: true},
		{name: "notice", fields: map[string]string{fieldPriority: "5"}, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := isWarningOrMoreSevere(tt.fields); got != tt.want {
				t.Fatalf("isWarningOrMoreSevere() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestJournalBootIDRemovesDashes(t *testing.T) {
	t.Parallel()

	got := journalBootID("467bc065-0ca7-4e21-9c4d-05ae34e3bc7c")
	want := "467bc0650ca74e219c4d05ae34e3bc7c"
	if got != want {
		t.Fatalf("journalBootID() = %q, want %q", got, want)
	}
}
