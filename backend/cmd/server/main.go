package main

import (
	"flag"
	"fmt"
	"log"
	"os"

	"opsweb/internal/api"
	"opsweb/internal/web"
)

var (
	version   = "dev"
	buildTime = "unknown"
)

func main() {
	addr := flag.String("addr", ":9800", "listen address")
	token := flag.String("token", "", "API access token (or env OPSWEB_TOKEN)")
	showVersion := flag.Bool("version", false, "print version and exit")
	flag.Parse()

	if *showVersion {
		fmt.Printf("opsweb %s (built %s)\n", version, buildTime)
		os.Exit(0)
	}

	t := *token
	if t == "" {
		t = os.Getenv("OPSWEB_TOKEN")
	}
	if t == "" {
		log.Fatal("token required: -token flag or OPSWEB_TOKEN env")
	}

	web.Init()
	api.Version = version + " (built " + buildTime + ")"
	r := api.NewRouter(t)
	log.Printf("opsweb %s listening on %s", version, *addr)
	if err := r.Run(*addr); err != nil {
		log.Fatal(err)
	}
}
