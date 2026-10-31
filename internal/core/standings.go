package core

type TeamStanding struct {
	Team   Team
	Seed   int
	Wins   int
	Losses int
	Streak string
}

type Conference struct {
	Name  string
	Teams []TeamStanding
}

type Standings struct {
	League      League
	Conferences []Conference
}

func (s Standings) IsEmpty() bool {
	for _, c := range s.Conferences {
		if len(c.Teams) > 0 {
			return false
		}
	}
	return true
}
