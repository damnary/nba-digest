package subscription

import (
	"context"
	"errors"
	"testing"

	"github.com/damnary/nba-digest/internal/core"
)

type fakeStore struct {
	nextID  core.SubscriberID
	chats   map[int64]core.SubscriberID
	subs    map[core.SubscriberID][]core.Subscription
	alerts  map[core.SubscriberID]bool
	deleted map[core.SubscriberID]bool
	teams   []core.Team
	failOn  string
}

func newFakeStore() *fakeStore {
	return &fakeStore{
		chats:   make(map[int64]core.SubscriberID),
		subs:    make(map[core.SubscriberID][]core.Subscription),
		alerts:  make(map[core.SubscriberID]bool),
		deleted: make(map[core.SubscriberID]bool),
		teams: []core.Team{
			{League: core.LeagueWNBA, Code: "SEA", Name: "Seattle Storm"},
			{League: core.LeagueWNBA, Code: "NYL", Name: "New York Liberty"},
			{League: core.LeagueWNBA, Code: "LVA", Name: "Las Vegas Aces"},
		},
	}
}

func (f *fakeStore) EnsureSubscriber(_ context.Context, chatID int64) (core.Subscriber, error) {
	if f.failOn == "ensure" {
		return core.Subscriber{}, errors.New("boom")
	}
	id, ok := f.chats[chatID]
	if !ok {
		f.nextID++
		id = f.nextID
		f.chats[chatID] = id
		f.alerts[id] = true
	}
	return core.Subscriber{ID: id, ChatID: chatID, AlertsOn: f.alerts[id]}, nil
}

func (f *fakeStore) DeleteSubscriber(_ context.Context, id core.SubscriberID) error {
	f.deleted[id] = true
	delete(f.subs, id)
	return nil
}

func (f *fakeStore) SubscriptionsOf(_ context.Context, id core.SubscriberID) ([]core.Subscription, error) {
	return f.subs[id], nil
}

func (f *fakeStore) AddSubscription(_ context.Context, id core.SubscriberID, league core.League, team core.TeamCode) error {
	f.subs[id] = append(f.subs[id], core.Subscription{SubscriberID: id, League: league, Team: team})
	return nil
}

func (f *fakeStore) RemoveSubscription(_ context.Context, id core.SubscriberID, league core.League, team core.TeamCode) error {
	kept := f.subs[id][:0]
	for _, s := range f.subs[id] {
		if s.League == league && s.Team == team {
			continue
		}
		kept = append(kept, s)
	}
	f.subs[id] = kept
	return nil
}

func (f *fakeStore) SetAlerts(_ context.Context, id core.SubscriberID, on bool) error {
	f.alerts[id] = on
	return nil
}

func (f *fakeStore) Teams(_ context.Context, league core.League) ([]core.Team, error) {
	if f.failOn == "teams" {
		return nil, errors.New("boom")
	}
	var out []core.Team
	for _, t := range f.teams {
		if t.League == league {
			out = append(out, t)
		}
	}
	return out, nil
}

func newService(t *testing.T) (*Service, *fakeStore) {
	t.Helper()
	store := newFakeStore()
	return New(store, core.LeagueWNBA), store
}

func selectedCodes(reply core.Reply) []core.TeamCode {
	var out []core.TeamCode
	for _, t := range reply.Selected() {
		out = append(out, t.Code)
	}
	return out
}

func TestStartOffersEveryTeamUnselected(t *testing.T) {
	svc, _ := newService(t)

	reply, err := svc.Handle(t.Context(), core.Command{ChatID: 1, Kind: core.CommandStart})
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	if reply.Kind != core.ReplyWelcome {
		t.Errorf("kind = %s", reply.Kind)
	}
	if len(reply.Teams) != 3 {
		t.Fatalf("want 3 options, got %d", len(reply.Teams))
	}
	if len(reply.Selected()) != 0 {
		t.Errorf("nothing should be selected yet: %v", selectedCodes(reply))
	}
}

func TestTeamOptionsAreSortedByCode(t *testing.T) {
	svc, _ := newService(t)

	reply, err := svc.Handle(t.Context(), core.Command{ChatID: 1, Kind: core.CommandTeams})
	if err != nil {
		t.Fatalf("teams: %v", err)
	}

	want := []core.TeamCode{"LVA", "NYL", "SEA"}
	for i, code := range want {
		if reply.Teams[i].Team.Code != code {
			t.Fatalf("option %d = %s, want %s", i, reply.Teams[i].Team.Code, code)
		}
	}
}

func TestToggleAddsThenRemoves(t *testing.T) {
	svc, store := newService(t)
	ctx := t.Context()
	toggle := core.Command{ChatID: 1, Kind: core.CommandToggleTeam, Team: "NYL"}

	added, err := svc.Handle(ctx, toggle)
	if err != nil {
		t.Fatalf("first toggle: %v", err)
	}
	if added.Kind != core.ReplyTeamAdded {
		t.Errorf("kind = %s, want team_added", added.Kind)
	}
	if added.Team.Name != "New York Liberty" {
		t.Errorf("reply should carry the team: %+v", added.Team)
	}
	if codes := selectedCodes(added); len(codes) != 1 || codes[0] != "NYL" {
		t.Errorf("selected = %v", codes)
	}

	removed, err := svc.Handle(ctx, toggle)
	if err != nil {
		t.Fatalf("second toggle: %v", err)
	}
	if removed.Kind != core.ReplyTeamRemoved {
		t.Errorf("kind = %s, want team_removed", removed.Kind)
	}
	if len(removed.Selected()) != 0 {
		t.Errorf("selected = %v", selectedCodes(removed))
	}
	if len(store.subs[1]) != 0 {
		t.Errorf("subscription left behind: %+v", store.subs[1])
	}
}

func TestToggleRejectsUnknownTeam(t *testing.T) {
	svc, store := newService(t)

	reply, err := svc.Handle(t.Context(), core.Command{ChatID: 1, Kind: core.CommandToggleTeam, Team: "ZZZ"})
	if err != nil {
		t.Fatalf("toggle: %v", err)
	}
	if reply.Kind != core.ReplyUnknownTeam {
		t.Errorf("kind = %s, want unknown_team", reply.Kind)
	}
	if len(store.subs[1]) != 0 {
		t.Error("an unknown team must not create a subscription")
	}
}

func TestSeveralTeamsCanBeSelected(t *testing.T) {
	svc, _ := newService(t)
	ctx := t.Context()

	for _, code := range []core.TeamCode{"NYL", "SEA"} {
		if _, err := svc.Handle(ctx, core.Command{ChatID: 1, Kind: core.CommandToggleTeam, Team: code}); err != nil {
			t.Fatalf("toggle %s: %v", code, err)
		}
	}

	reply, err := svc.Handle(ctx, core.Command{ChatID: 1, Kind: core.CommandTeams})
	if err != nil {
		t.Fatalf("teams: %v", err)
	}
	codes := selectedCodes(reply)
	if len(codes) != 2 || codes[0] != "NYL" || codes[1] != "SEA" {
		t.Errorf("selected = %v, want [NYL SEA]", codes)
	}
}

func TestAlertsToggle(t *testing.T) {
	svc, store := newService(t)
	ctx := t.Context()

	reply, err := svc.Handle(ctx, core.Command{ChatID: 1, Kind: core.CommandAlerts, Enable: false})
	if err != nil {
		t.Fatalf("alerts off: %v", err)
	}
	if reply.Kind != core.ReplyAlerts || reply.Enabled {
		t.Errorf("unexpected reply: %+v", reply)
	}
	if store.alerts[1] {
		t.Error("alerts should be off in the store")
	}

	if _, err := svc.Handle(ctx, core.Command{ChatID: 1, Kind: core.CommandAlerts, Enable: true}); err != nil {
		t.Fatalf("alerts on: %v", err)
	}
	if !store.alerts[1] {
		t.Error("alerts should be back on")
	}
}

func TestStopDeletesTheSubscriber(t *testing.T) {
	svc, store := newService(t)
	ctx := t.Context()

	if _, err := svc.Handle(ctx, core.Command{ChatID: 1, Kind: core.CommandToggleTeam, Team: "NYL"}); err != nil {
		t.Fatalf("toggle: %v", err)
	}

	reply, err := svc.Handle(ctx, core.Command{ChatID: 1, Kind: core.CommandStop})
	if err != nil {
		t.Fatalf("stop: %v", err)
	}
	if reply.Kind != core.ReplyStopped {
		t.Errorf("kind = %s", reply.Kind)
	}
	if !store.deleted[1] {
		t.Error("subscriber was not deleted")
	}
}

func TestHelpDoesNotTouchTheStore(t *testing.T) {
	svc, store := newService(t)
	store.failOn = "ensure"

	for _, kind := range []core.CommandKind{core.CommandHelp, core.CommandUnknown} {
		reply, err := svc.Handle(t.Context(), core.Command{ChatID: 1, Kind: kind})
		if err != nil {
			t.Fatalf("%s: %v", kind, err)
		}
		if reply.Kind != core.ReplyHelp {
			t.Errorf("%s produced %s", kind, reply.Kind)
		}
	}
}

func TestStoreFailureIsWrapped(t *testing.T) {
	svc, store := newService(t)
	store.failOn = "teams"

	if _, err := svc.Handle(t.Context(), core.Command{ChatID: 1, Kind: core.CommandStart}); err == nil {
		t.Fatal("want an error when the store fails")
	}
}
