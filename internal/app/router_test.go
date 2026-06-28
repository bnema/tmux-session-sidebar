package app

import "testing"

func TestDirectRouteMutatesSidebarIncludesKillAction(t *testing.T) {
	if !directRouteMutatesSidebar("action/kill") {
		t.Fatal("action/kill should be serialized because killing the current session can switch the client and move sidebars")
	}
}
