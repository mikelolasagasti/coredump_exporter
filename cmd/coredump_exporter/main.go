package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

const defaultListenAddress = ":9113"

var (
	version = "dev"
	date    = "unknown"
)

func main() {
	listenAddress := flag.String("web.listen-address", defaultListenAddress, "Address on which to expose metrics and web interface.")
	showVersion := flag.Bool("version", false, "Print version information and exit.")
	flag.Parse()
	if *showVersion {
		fmt.Println(versionString())
		return
	}

	registry := prometheus.NewRegistry()
	registry.MustRegister(
		collectors.NewGoCollector(),
		collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}),
		newCoredumpCollector(newSystemdJournalReader(), readBootID),
	)

	http.Handle("/metrics", promhttp.HandlerFor(registry, promhttp.HandlerOpts{}))
	http.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = fmt.Fprintln(w, `<html><head><title>Coredump Exporter</title></head><body><h1>Coredump Exporter</h1><p><a href="/metrics">Metrics</a></p></body></html>`)
	})

	server := &http.Server{
		Addr:              *listenAddress,
		ReadHeaderTimeout: 5 * time.Second,
	}

	log.Printf("listening on %s", *listenAddress)
	log.Fatal(server.ListenAndServe())
}

func versionString() string {
	return fmt.Sprintf("coredump_exporter version=%s date=%s", version, date)
}
