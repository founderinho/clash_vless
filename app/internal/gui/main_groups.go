//go:build !android && cgo

package gui

import (
	"net"
	"net/url"
	"slices"

	"clashvless/internal/store"
)

type mainGroup struct {
	indices []int
	mains   []store.Main
}

// Only the server address and display name may differ; credentials, port and
// every transport/security parameter must match. Never resolve DDNS to group it.
func mainGroupKey(link string) string {
	u, err := url.Parse(link)
	if err != nil || u.Scheme != "vless" || u.User == nil || u.User.Username() == "" || u.Hostname() == "" || u.Port() == "" {
		return link
	}
	q, err := url.ParseQuery(u.RawQuery)
	if err != nil {
		return link
	}
	u.Host = net.JoinHostPort("server", u.Port())
	u.Fragment, u.RawFragment = "", ""
	u.RawQuery = q.Encode()
	return u.String()
}

func groupMains(mains []store.Main) []mainGroup {
	var groups []mainGroup
	byKey := make(map[string]int)
	for i, main := range mains {
		key := mainGroupKey(main.URL)
		g, found := byKey[key]
		if !found {
			g = len(groups)
			byKey[key] = g
			groups = append(groups, mainGroup{})
		}
		groups[g].indices = append(groups[g].indices, i)
		groups[g].mains = append(groups[g].mains, main)
	}
	return groups
}

func moveMainGroup(slot *store.Slot, mainIndex, delta int) {
	groups := groupMains(slot.Mains)
	for from, group := range groups {
		if !slices.Contains(group.indices, mainIndex) {
			continue
		}
		to := from + delta
		if to < 0 || to >= len(groups) {
			return
		}
		groups[from], groups[to] = groups[to], groups[from]
		var mains []store.Main
		for _, g := range groups {
			mains = append(mains, g.mains...)
		}
		slot.Mains = mains
		return
	}
}
