package main

import (
	"fmt"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/vchimishuk/config"
	"github.com/vchimishuk/opt"
	"github.com/vchimishuk/warden/slices"
	xslices "golang.org/x/exp/slices"
)

const (
	Version       = "0.1.0"
	DefaultConfig = "/etc/warden.conf"
)

type WardenHTTP struct {
	Warden *Warden
}

// Supported API:
// GET /hosts
//   Returns list of online hosts.
// POST /hosts/{hostname}
//   Send heartbeat for host specified by {hostname}.
//
// TODO: Support X-Forwarded-For
// TODO: Support DELETE /hosts/{hostname}
func (h *WardenHTTP) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain")

	segs := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	segs = slices.Remove(segs, func(s string) bool {
		return s == ""
	})

	if len(segs) == 0 {
		if r.Method != http.MethodGet {
			http.NotFound(w, r)
			return
		}
		http.Redirect(w, r, "/hosts", http.StatusSeeOther)
		return
	}

	if len(segs) == 1 {
		if r.Method != http.MethodGet {
			http.NotFound(w, r)
			return
		}

		hosts := h.Warden.Hosts()
		xslices.SortFunc(hosts, func(a, b *Host) bool {
			return a.Name < b.Name
		})
		for _, h := range hosts {
			fmt.Fprintf(w, "%s %s %s\n", h.Name, h.IP, h.Heartbeat.Format(time.RFC3339))
		}
		return
	}

	if len(segs) == 2 {
		if r.Method != http.MethodPost {
			http.NotFound(w, r)
			return
		}

		ip, _, err := net.SplitHostPort(r.RemoteAddr)
		if err != nil {
			panic(err)
		}
		h.Warden.Heartbeat(segs[1], ip)
		return
	}

	http.NotFound(w, r)
}

func parseConfig(path string) (*config.Config, error) {
	blockProps := []*config.PropertySpec{
		&config.PropertySpec{
			Type:    config.TypeStringList,
			Name:    "hosts",
			Repeat:  false,
			Require: false,
		},
		&config.PropertySpec{
			Type:    config.TypeString,
			Name:    "exec",
			Repeat:  false,
			Require: true,
		},
	}
	spec := &config.Spec{
		Properties: []*config.PropertySpec{
			&config.PropertySpec{
				Type:    config.TypeDuration,
				Name:    "heartbeat-ttl",
				Repeat:  false,
				Require: true,
			},
			&config.PropertySpec{
				Type:    config.TypeString,
				Name:    "address",
				Repeat:  false,
				Require: true,
			},
			&config.PropertySpec{
				Type:    config.TypeInt,
				Name:    "port",
				Repeat:  false,
				Require: true,
			},
		},
		Blocks: []*config.BlockSpec{
			&config.BlockSpec{
				Name:       "online",
				Repeat:     true,
				Require:    false,
				Properties: blockProps,
			},
			&config.BlockSpec{
				Name:       "online-all",
				Repeat:     true,
				Require:    false,
				Properties: blockProps,
			},
			&config.BlockSpec{
				Name:       "offline",
				Repeat:     true,
				Require:    false,
				Properties: blockProps,
			},
			&config.BlockSpec{
				Name:       "offline-all",
				Repeat:     true,
				Require:    false,
				Properties: blockProps,
			},
		},
	}

	b, err := ioutil.ReadFile(path)
	if err != nil {
		return nil,
			fmt.Errorf("failed to read `%s`: %w", path, err)
	}
	cfg, err := config.Parse(spec, string(b))
	if err != nil {
		return nil, fmt.Errorf("failed to parse `%s`: %w", path, err)
	}

	return cfg, nil
}

func execute(cmd string) {
	log.Printf("Executing external command `%s`...", cmd)

	c := exec.Command("sh", "-c", cmd)
	c.Dir = os.TempDir()
	out, err := c.CombinedOutput()
	if err != nil {
		log.Printf("External command failed: %s. Output: %s",
			err.Error(), out)
	} else {
		log.Printf("External command exited successfuly. Output: %s",
			out)
	}
}

func main() {
	optDescs := []*opt.Desc{
		{"c", "config", opt.ArgString, "FILE",
			"configuration file name"},
		{"h", "help", opt.ArgNone, "",
			"display this help and exit"},
		{"v", "version", opt.ArgNone, "",
			"output version information and exit"},
	}
	opts, _, err := opt.Parse(os.Args[1:], optDescs)
	if err != nil {
		log.Fatalf("%s", err)
	}
	if opts.Bool("help") {
		fmt.Println("Usage: warden [OPTION]...")
		fmt.Println()
		fmt.Println("Available optional options:")
		fmt.Print(opt.Usage(optDescs))
		os.Exit(0)
	}
	if opts.Bool("version") {
		fmt.Printf("%s %s", os.Args[0], Version)
		os.Exit(0)
	}

	cfg, err := parseConfig(opts.StringOr("config", DefaultConfig))
	if err != nil {
		log.Fatalf(err.Error())
	}

	w := NewWarden(cfg.Duration("heartbeat-ttl"))
	for _, b := range cfg.Blocks {
		cmd := b.String("exec")
		f := func(e Event, host *Host, online []*Host) {
			execute(cmd)
		}

		switch b.Name {
		case string(EventOffline):
			w.Register(EventOffline, nil, f)
		case string(EventOfflineAll):
			w.Register(EventOfflineAll, nil, f)
		case string(EventOnline):
			w.Register(EventOnline, nil, f)
		case string(EventOnlineAll):
			w.Register(EventOnlineAll, nil, f)
		}
	}

	addr := fmt.Sprintf("%s:%d", cfg.String("address"), cfg.Int("port"))
	srv := &http.Server{
		Addr:    addr,
		Handler: &WardenHTTP{w},
	}

	log.Printf("starting API handler on http://%s", addr)
	srv.ListenAndServe()
	// TODO: Server shutdown.
}
