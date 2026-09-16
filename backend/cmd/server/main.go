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
	user := flag.String("user", "", "initial login username (or env OPSWEB_USER)")
	pass := flag.String("pass", "", "initial login password (or env OPSWEB_PASS)")
	dsn := flag.String("dsn", "", "pgsql DSN (or env OPSWEB_DSN), e.g. postgres://opsweb:pw@127.0.0.1:5433/opsweb")
	showVersion := flag.Bool("version", false, "print version and exit")
	flag.Parse()

	if *showVersion {
		fmt.Printf("opsweb %s (built %s)\n", version, buildTime)
		os.Exit(0)
	}

	u := *user
	if u == "" {
		u = os.Getenv("OPSWEB_USER")
	}
	p := *pass
	if p == "" {
		p = os.Getenv("OPSWEB_PASS")
	}
	d := *dsn
	if d == "" {
		d = os.Getenv("OPSWEB_DSN")
	}
	if d == "" {
		log.Fatal("pgsql DSN required: -dsn flag or OPSWEB_DSN env")
	}
	if err := api.InitStore(d, u, p); err != nil {
		log.Fatalf("init store: %v", err)
	}
	defer api.CloseStore()

	web.Init()
	api.Version = version + " (built " + buildTime + ")"
	r := api.NewRouter()
	log.Printf("opsweb %s listening on %s", version, *addr)
	if err := r.Run(*addr); err != nil {
		log.Fatal(err)
	}
}
