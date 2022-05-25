package main

import (
	"sync"
	"time"

	"github.com/vchimishuk/warden/slices"
)

type Host struct {
	Name      string
	IP        string
	Heartbeat time.Time
}

type Event string

const (
	EventOffline    Event = "offline"
	EventOfflineAll Event = "offline-all"
	EventOnline     Event = "online"
	EventOnlineAll  Event = "online-all"
)

type Handler func(e Event, host *Host, online []*Host)

type EventHandler struct {
	Event   Event
	Hosts   []string
	Handler Handler
}

type Warden struct {
	hbTTL    time.Duration
	mu       sync.Mutex
	hosts    []*Host
	handlers []*EventHandler
}

func NewWarden(hbTTL time.Duration) *Warden {
	w := &Warden{hbTTL: hbTTL}

	go func() {
		for {
			now := time.Now()
			next := now.Add(hbTTL)

			w.mu.Lock()

			for i := 0; i < len(w.hosts); {
				h := w.hosts[i]
				if !h.Heartbeat.Add(hbTTL).After(now) {
					w.hosts = append(w.hosts[:i],
						w.hosts[i+1:]...)
					w.event(false, h)
				} else {
					i++
				}
			}

			for _, h := range w.hosts {
				exp := h.Heartbeat.Add(hbTTL)
				if exp.Before(next) {
					next = exp
				}
			}
			w.mu.Unlock()
			time.Sleep(next.Sub(time.Now()))
		}
	}()

	return w
}

func (w *Warden) Heartbeat(host string, ip string) {
	w.mu.Lock()
	defer w.mu.Unlock()

	h := w.findHost(host)
	if h == nil {
		h = &Host{host, ip, time.Now()}
		w.event(true, h)
		w.hosts = append(w.hosts, h)
	} else {
		h.IP = ip
		h.Heartbeat = time.Now()
	}
}

func (w *Warden) Hosts() []*Host {
	w.mu.Lock()
	defer w.mu.Unlock()

	return w.hostsCopy()
}

func (w *Warden) Register(event Event, hosts []string, handler Handler) {
	w.mu.Lock()
	defer w.mu.Unlock()

	w.handlers = append(w.handlers, &EventHandler{event, hosts, handler})
}

func (w *Warden) event(online bool, host *Host) {
	onhosts := w.hostsCopy()

	for _, h := range w.handlers {
		switch online {
		case true:
			switch h.Event {
			case EventOnline:
				if len(h.Hosts) == 0 ||
					contains(h.Hosts, host.Name) {
					go h.Handler(EventOnline,
						host, onhosts)
				}
			case EventOnlineAll:
				var all []*Host
				all = append(all, onhosts...)
				all = append(all, host)
				if len(h.Hosts) == 0 ||
					containsAll(all, h.Hosts) {
					go h.Handler(EventOnlineAll,
						host, onhosts)
				}
			}
		case false:
			switch h.Event {
			case EventOffline:
				if len(h.Hosts) == 0 ||
					contains(h.Hosts, host.Name) {
					go h.Handler(EventOffline,
						host, onhosts)
				}
			case EventOfflineAll:
				if len(h.Hosts) == 0 && len(onhosts) == 0 {
					go h.Handler(EventOffline,
						host, onhosts)
				} else {
					if containsNone(onhosts, h.Hosts) &&
						contains(h.Hosts, host.Name) {
						go h.Handler(EventOfflineAll,
							host, onhosts)
					}
				}
			}
		}
	}
}

func (w *Warden) hostsCopy() []*Host {
	var hosts []*Host
	for _, h := range w.hosts {
		hosts = append(hosts, &Host{h.Name, h.IP, h.Heartbeat})
	}

	return hosts
}

func (w *Warden) findHost(name string) *Host {
	for _, h := range w.hosts {
		if h.Name == name {
			return h
		}
	}

	return nil
}

func contains(haystack []string, needle string) bool {
	return slices.Contains(haystack, func(e string) bool {
		return e == needle
	})
}

func containsAll(haystack []*Host, needles []string) bool {
	for _, n := range needles {
		c := slices.Contains(haystack, func(h *Host) bool {
			return h.Name == n
		})
		if !c {
			return false
		}
	}

	return true
}

func containsNone(haystack []*Host, needles []string) bool {
	for _, n := range needles {
		c := slices.Contains(haystack, func(h *Host) bool {
			return h.Name == n
		})
		if c {
			return false
		}
	}

	return true
}
