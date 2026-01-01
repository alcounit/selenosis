package selenium

type Status struct {
	Value Value `json:"value"`
}

func (s *Status) Set(msg string, ready bool) {
	s.Value = map[string]any{
		"message": msg,
		"ready":   ready,
	}
}
