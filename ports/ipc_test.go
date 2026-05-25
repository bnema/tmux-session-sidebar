package ports

import "testing"

func TestSidebarIPCRequests(t *testing.T) {
	tests := []struct {
		name   string
		got    Request
		kind   string
		client string
		args   map[string]string
	}{
		{name: "open", got: SidebarOpenRequest("%1"), kind: IPCSidebarOpen, client: "%1"},
		{name: "close", got: SidebarCloseRequest("%1"), kind: IPCSidebarClose, client: "%1"},
		{name: "toggle", got: SidebarToggleRequest("%1"), kind: IPCSidebarToggle, client: "%1"},
		{name: "refresh", got: SidebarRefreshRequest("%1"), kind: IPCSidebarRefresh, client: "%1"},
		{name: "active client", got: ActiveClientRequest("%1"), kind: IPCActiveClient, client: "%1"},
		{name: "health", got: HealthRequest(), kind: IPCHealth},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.got.Kind != tt.kind {
				t.Fatalf("Kind = %q, want %q", tt.got.Kind, tt.kind)
			}
			if tt.got.ClientID != tt.client {
				t.Fatalf("ClientID = %q, want %q", tt.got.ClientID, tt.client)
			}
			if len(tt.args) == 0 && len(tt.got.Args) != 0 {
				t.Fatalf("Args = %#v, want empty", tt.got.Args)
			}
		})
	}
}

func TestSidebarIPCRequestWithArgsCopiesMap(t *testing.T) {
	args := map[string]string{"session": "alpha"}
	req := SidebarRequest(IPCSidebarOpen, "%1", args)
	args["session"] = "beta"

	if req.Args["session"] != "alpha" {
		t.Fatalf("request args were aliased: %#v", req.Args)
	}
}
