package reflection

type TurnReflection struct {
	EmpathyWorked   bool         `json:"empathy_worked"`
	UserShortReply  bool         `json:"user_short_reply"`
	PreferredLength string       `json:"preferred_length"`
	StyleNote       string       `json:"style_note"`
	TabooHit        bool         `json:"taboo_hit"`
	TabooNote       string       `json:"taboo_note"`
	BriefUpdates    []BriefDelta `json:"brief_updates"`
	BondNickname    string       `json:"bond_nickname"`
	InsideJoke      string       `json:"inside_joke"`
}

type BriefDelta struct {
	Category   string  `json:"category"`
	Content    string  `json:"content"`
	Importance float32 `json:"importance"`
}
