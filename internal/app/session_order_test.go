package app

import (
	"reflect"
	"testing"
)

func TestApplySessionOrderPrioritizesSavedOrderAndAppendsNewSessions(t *testing.T) {
	live := []string{"alpha", "beta", "gamma", "delta"}
	order := []string{"gamma", "missing", "alpha"}

	got := applySessionOrder(live, order)
	want := []string{"gamma", "alpha", "beta", "delta"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("applySessionOrder() = %#v, want %#v", got, want)
	}
}

func TestMoveSessionOrderMovesSelectedSessionAmongLiveSessions(t *testing.T) {
	live := []string{"alpha", "beta", "gamma"}
	order := []string{"gamma", "alpha", "beta"}

	got := moveSessionOrder(live, order, "alpha", 1)
	want := []string{"gamma", "beta", "alpha"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("moveSessionOrder() = %#v, want %#v", got, want)
	}
}

func TestMoveSessionOrderClampsAtEdges(t *testing.T) {
	live := []string{"alpha", "beta", "gamma"}

	got := moveSessionOrder(live, nil, "alpha", -1)
	want := []string{"alpha", "beta", "gamma"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("move first up = %#v, want %#v", got, want)
	}

	got = moveSessionOrder(live, nil, "gamma", 1)
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("move last down = %#v, want %#v", got, want)
	}
}
