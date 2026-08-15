package subscription

import (
	"cmp"
	"context"
	"fmt"
	"slices"

	"github.com/damnary/nba-digest/internal/core"
)

type Store interface {
	EnsureSubscriber(ctx context.Context, chatID int64) (core.Subscriber, error)
	DeleteSubscriber(ctx context.Context, id core.SubscriberID) error
	SubscriptionsOf(ctx context.Context, id core.SubscriberID) ([]core.Subscription, error)
	AddSubscription(ctx context.Context, id core.SubscriberID, league core.League, team core.TeamCode) error
	RemoveSubscription(ctx context.Context, id core.SubscriberID, league core.League, team core.TeamCode) error
	SetAlerts(ctx context.Context, id core.SubscriberID, on bool) error
	Teams(ctx context.Context, league core.League) ([]core.Team, error)
}

type Service struct {
	store  Store
	league core.League
}

func New(store Store, league core.League) *Service {
	return &Service{store: store, league: league}
}

func (s *Service) Handle(ctx context.Context, cmd core.Command) (core.Reply, error) {
	league := cmd.League
	if league == "" {
		league = s.league
	}

	if cmd.Kind == core.CommandHelp || cmd.Kind == core.CommandUnknown {
		return core.Reply{Kind: core.ReplyHelp}, nil
	}

	sub, err := s.store.EnsureSubscriber(ctx, cmd.ChatID)
	if err != nil {
		return core.Reply{}, fmt.Errorf("ensure subscriber: %w", err)
	}

	switch cmd.Kind {
	case core.CommandStart:
		options, err := s.options(ctx, sub.ID, league)
		if err != nil {
			return core.Reply{}, err
		}
		return core.Reply{Kind: core.ReplyWelcome, Teams: options}, nil

	case core.CommandTeams:
		options, err := s.options(ctx, sub.ID, league)
		if err != nil {
			return core.Reply{}, err
		}
		return core.Reply{Kind: core.ReplyTeams, Teams: options}, nil

	case core.CommandToggleTeam:
		return s.toggle(ctx, sub.ID, league, cmd.Team)

	case core.CommandAlerts:
		if err := s.store.SetAlerts(ctx, sub.ID, cmd.Enable); err != nil {
			return core.Reply{}, fmt.Errorf("set alerts: %w", err)
		}
		return core.Reply{Kind: core.ReplyAlerts, Enabled: cmd.Enable}, nil

	case core.CommandStop:
		if err := s.store.DeleteSubscriber(ctx, sub.ID); err != nil {
			return core.Reply{}, fmt.Errorf("delete subscriber: %w", err)
		}
		return core.Reply{Kind: core.ReplyStopped}, nil
	}

	return core.Reply{Kind: core.ReplyHelp}, nil
}

func (s *Service) toggle(ctx context.Context, id core.SubscriberID, league core.League, code core.TeamCode) (core.Reply, error) {
	teams, err := s.store.Teams(ctx, league)
	if err != nil {
		return core.Reply{}, fmt.Errorf("teams: %w", err)
	}

	idx := slices.IndexFunc(teams, func(t core.Team) bool { return t.Code == code })
	if idx < 0 {
		return core.Reply{Kind: core.ReplyUnknownTeam}, nil
	}
	team := teams[idx]

	current, err := s.store.SubscriptionsOf(ctx, id)
	if err != nil {
		return core.Reply{}, fmt.Errorf("subscriptions: %w", err)
	}

	subscribed := slices.ContainsFunc(current, func(sub core.Subscription) bool {
		return sub.League == league && sub.Team == code
	})

	kind := core.ReplyTeamAdded
	if subscribed {
		kind = core.ReplyTeamRemoved
		err = s.store.RemoveSubscription(ctx, id, league, code)
	} else {
		err = s.store.AddSubscription(ctx, id, league, code)
	}
	if err != nil {
		return core.Reply{}, fmt.Errorf("toggle subscription: %w", err)
	}

	options, err := s.options(ctx, id, league)
	if err != nil {
		return core.Reply{}, err
	}
	return core.Reply{Kind: kind, Team: team, Teams: options}, nil
}

func (s *Service) options(ctx context.Context, id core.SubscriberID, league core.League) ([]core.TeamOption, error) {
	teams, err := s.store.Teams(ctx, league)
	if err != nil {
		return nil, fmt.Errorf("teams: %w", err)
	}

	current, err := s.store.SubscriptionsOf(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("subscriptions: %w", err)
	}

	selected := make(map[core.TeamCode]bool, len(current))
	for _, sub := range current {
		if sub.League == league {
			selected[sub.Team] = true
		}
	}

	options := make([]core.TeamOption, 0, len(teams))
	for _, team := range teams {
		options = append(options, core.TeamOption{Team: team, Selected: selected[team.Code]})
	}
	slices.SortStableFunc(options, func(a, b core.TeamOption) int {
		return cmp.Compare(a.Team.Code, b.Team.Code)
	})
	return options, nil
}
