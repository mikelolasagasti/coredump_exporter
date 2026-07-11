package main

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync/atomic"

	"github.com/coreos/go-systemd/v22/sdjournal"
	"github.com/prometheus/client_golang/prometheus"
)

const (
	fieldBootID         = "_BOOT_ID"
	fieldMessageID      = "MESSAGE_ID"
	fieldPriority       = "PRIORITY"
	fieldCoredumpExe    = "COREDUMP_EXE"
	fieldCoredumpSignal = "COREDUMP_SIGNAL_NAME"
	// messageIDCoredump is systemd-coredump's MESSAGE_ID. Matching it in journald avoids scanning unrelated warning/error entries.
	messageIDCoredump = "fc2e22bc6ee647b6b90729ab34a250b1"
	// systemd/syslog priorities are 0=emerg through 7=debug; 4 is warning.
	maxJournalPriority   = 4
	malformedMissingExe  = "missing_exe"
	malformedEmptyExe    = "empty_exe"
	malformedMissingSig  = "missing_signal"
	malformedEmptySignal = "empty_signal"
)

// bootIDPath is a package variable so readBootID can be tested without /proc.
var bootIDPath = "/proc/sys/kernel/random/boot_id"

type journalEntry map[string]string

type journalReader interface {
	Entries(bootID string) ([]journalEntry, error)
}

type bootIDReader func() (string, error)

type coredumpCollector struct {
	reader     journalReader
	bootID     bootIDReader
	scrapeErrs atomic.Uint64

	countDesc         *prometheus.Desc
	countBySignalDesc *prometheus.Desc
	malformedDesc     *prometheus.Desc
	scrapeErrsTotal   *prometheus.Desc
}

func newCoredumpCollector(reader journalReader, bootID bootIDReader) *coredumpCollector {
	return &coredumpCollector{
		reader: reader,
		bootID: bootID,
		countDesc: prometheus.NewDesc(
			"coredump_count",
			"Number of valid coredump journal entries for the current boot.",
			[]string{"boot_id"}, nil,
		),
		countBySignalDesc: prometheus.NewDesc(
			"coredump_count_by_executable_signal",
			"Number of valid coredump journal entries grouped by executable and signal for the current boot.",
			[]string{"boot_id", "exe", "signal"}, nil,
		),
		malformedDesc: prometheus.NewDesc(
			"coredump_malformed_entries",
			"Number of coredump-like journal entries skipped because required fields were missing or invalid.",
			[]string{"boot_id", "reason"}, nil,
		),
		scrapeErrsTotal: prometheus.NewDesc(
			"coredump_scrape_errors_total",
			"Total number of failed journal scrapes.",
			nil, nil,
		),
	}
}

func (c *coredumpCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- c.countDesc
	ch <- c.countBySignalDesc
	ch <- c.malformedDesc
	ch <- c.scrapeErrsTotal
}

func (c *coredumpCollector) Collect(ch chan<- prometheus.Metric) {
	bootID, err := c.bootID()
	if err != nil {
		c.scrapeErrs.Add(1)
		ch <- prometheus.MustNewConstMetric(c.scrapeErrsTotal, prometheus.CounterValue, float64(c.scrapeErrs.Load()))
		ch <- prometheus.NewInvalidMetric(c.countDesc, fmt.Errorf("read boot ID: %w", err))
		return
	}

	entries, err := c.reader.Entries(bootID)
	if err != nil {
		c.scrapeErrs.Add(1)
		ch <- prometheus.MustNewConstMetric(c.scrapeErrsTotal, prometheus.CounterValue, float64(c.scrapeErrs.Load()))
		ch <- prometheus.NewInvalidMetric(c.countDesc, fmt.Errorf("read journal: %w", err))
		return
	}

	count, grouped, malformed := summarizeEntries(entries)
	ch <- prometheus.MustNewConstMetric(c.countDesc, prometheus.GaugeValue, float64(count), bootID)

	for group, value := range grouped {
		ch <- prometheus.MustNewConstMetric(c.countBySignalDesc, prometheus.GaugeValue, float64(value), bootID, group.exe, group.signal)
	}

	for reason, value := range malformed {
		ch <- prometheus.MustNewConstMetric(c.malformedDesc, prometheus.GaugeValue, float64(value), bootID, reason)
	}

	ch <- prometheus.MustNewConstMetric(c.scrapeErrsTotal, prometheus.CounterValue, float64(c.scrapeErrs.Load()))
}

type coredumpGroup struct {
	exe    string
	signal string
}

func summarizeEntries(entries []journalEntry) (int, map[coredumpGroup]int, map[string]int) {
	grouped := map[coredumpGroup]int{}
	malformed := map[string]int{
		malformedMissingExe:  0,
		malformedEmptyExe:    0,
		malformedMissingSig:  0,
		malformedEmptySignal: 0,
	}

	count := 0
	for _, entry := range entries {
		exe, ok := entry[fieldCoredumpExe]
		if !ok {
			malformed[malformedMissingExe]++
			continue
		}
		exe = strings.TrimSpace(exe)
		if exe == "" {
			malformed[malformedEmptyExe]++
			continue
		}

		signal, ok := entry[fieldCoredumpSignal]
		if !ok {
			malformed[malformedMissingSig]++
			continue
		}
		signal = strings.TrimSpace(signal)
		if signal == "" {
			malformed[malformedEmptySignal]++
			continue
		}

		count++
		grouped[coredumpGroup{exe: exe, signal: signal}]++
	}

	return count, grouped, malformed
}

func readBootID() (string, error) {
	data, err := os.ReadFile(bootIDPath)
	if err != nil {
		return "", err
	}
	bootID := strings.TrimSpace(string(data))
	if bootID == "" {
		return "", fmt.Errorf("empty boot ID")
	}
	return bootID, nil
}

type systemdJournalReader struct{}

func newSystemdJournalReader() systemdJournalReader {
	return systemdJournalReader{}
}

func (systemdJournalReader) Entries(bootID string) ([]journalEntry, error) {
	j, err := sdjournal.NewJournal()
	if err != nil {
		return nil, fmt.Errorf("open journal: %w", err)
	}
	defer func() {
		_ = j.Close()
	}()

	if err := j.AddMatch(fieldMessageID + "=" + messageIDCoredump); err != nil {
		return nil, fmt.Errorf("add message ID match: %w", err)
	}
	if err := j.AddMatch(fieldBootID + "=" + journalBootID(bootID)); err != nil {
		return nil, fmt.Errorf("add boot ID match: %w", err)
	}
	if err := j.SeekHead(); err != nil {
		return nil, fmt.Errorf("seek journal head: %w", err)
	}

	var entries []journalEntry
	for {
		n, err := j.Next()
		if err != nil {
			return nil, fmt.Errorf("read next journal entry: %w", err)
		}
		if n == 0 {
			break
		}

		entry, err := j.GetEntry()
		if err != nil {
			return nil, fmt.Errorf("get journal entry: %w", err)
		}

		if !isWarningOrMoreSevere(entry.Fields) {
			continue
		}

		entries = append(entries, journalEntry(entry.Fields))
	}

	return entries, nil
}

// journalBootID returns the dashless format used by journald's _BOOT_ID field.
func journalBootID(bootID string) string {
	return strings.ReplaceAll(bootID, "-", "")
}

// isWarningOrMoreSevere keeps entries without a parseable PRIORITY rather than dropping potentially valid coredumps.
func isWarningOrMoreSevere(fields map[string]string) bool {
	priority, ok := fields[fieldPriority]
	if !ok {
		return true
	}

	parsed, err := strconv.Atoi(priority)
	if err != nil {
		return true
	}

	return parsed <= maxJournalPriority
}
