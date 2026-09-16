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
	user := flag.String("user", "", "login username (or env OPSWEB_USER)")
	pass := flag.String("pass", "", "login password (or env OPSWEB_PASS)")
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
	if u == "" || p == "" {
		log.Fatal("credentials required: -user/-pass flags or OPSWEB_USER/OPSWEB_PASS env")
	}

	web.Init()
	api.Version = version + " (built " + buildTime + ")"
	r := api.NewRouter(u, p)
	log.Printf("opsweb %s listening on %s", version, *addr)
	if err := r.Run(*addr); err != nil {
		log.Fatal(err)
	}
}
