package espn

type scoreboardResponse struct {
	Events []struct {
		ID           string `json:"id"`
		Date         string `json:"date"`
		Competitions []struct {
			Status struct {
				Period       int    `json:"period"`
				DisplayClock string `json:"displayClock"`
				Type         struct {
					Name      string `json:"name"`
					State     string `json:"state"`
					Completed bool   `json:"completed"`
				} `json:"type"`
			} `json:"status"`
			Competitors []struct {
				HomeAway string `json:"homeAway"`
				Score    string `json:"score"`
				Team     struct {
					ID           string `json:"id"`
					Abbreviation string `json:"abbreviation"`
					DisplayName  string `json:"displayName"`
				} `json:"team"`
			} `json:"competitors"`
		} `json:"competitions"`
	} `json:"events"`
}

type teamsResponse struct {
	Sports []struct {
		Leagues []struct {
			Teams []struct {
				Team struct {
					ID           string `json:"id"`
					Abbreviation string `json:"abbreviation"`
					DisplayName  string `json:"displayName"`
				} `json:"team"`
			} `json:"teams"`
		} `json:"leagues"`
	} `json:"sports"`
}

type playsResponse struct {
	Count     int `json:"count"`
	PageIndex int `json:"pageIndex"`
	PageSize  int `json:"pageSize"`
	PageCount int `json:"pageCount"`
	Items     []struct {
		ID             string `json:"id"`
		SequenceNumber string `json:"sequenceNumber"`
		ScoringPlay    bool   `json:"scoringPlay"`
		ScoreValue     int    `json:"scoreValue"`
		HomeScore      int    `json:"homeScore"`
		AwayScore      int    `json:"awayScore"`
		Wallclock      string `json:"wallclock"`
		Period         struct {
			Number int `json:"number"`
		} `json:"period"`
		Clock struct {
			Value        float64 `json:"value"`
			DisplayValue string  `json:"displayValue"`
		} `json:"clock"`
		Team struct {
			Ref string `json:"$ref"`
		} `json:"team"`
		Participants []struct {
			Athlete struct {
				Ref string `json:"$ref"`
			} `json:"athlete"`
		} `json:"participants"`
	} `json:"items"`
}

type summaryResponse struct {
	Boxscore struct {
		Players []struct {
			Team struct {
				ID           string `json:"id"`
				Abbreviation string `json:"abbreviation"`
			} `json:"team"`
			Statistics []struct {
				Names    []string `json:"names"`
				Athletes []struct {
					Athlete struct {
						ID          string `json:"id"`
						DisplayName string `json:"displayName"`
					} `json:"athlete"`
					Stats []string `json:"stats"`
				} `json:"athletes"`
			} `json:"statistics"`
		} `json:"players"`
	} `json:"boxscore"`
}
