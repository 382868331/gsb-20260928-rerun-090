// Package seed ships a built-in story used for first run and end-to-end checks.
package seed

import "storyplatform/internal/story"

func fact(k, op string, v interface{}) *story.Condition {
	return &story.Condition{Fact: k, Op: op, Value: v}
}

// HeroV1 is the initial version of the sample story.
func HeroV1() *story.Definition {
	return &story.Definition{
		ID:      "hero-road",
		Title:   "Hero Road",
		Version: "1",
		Start:   "start",
		Nodes: map[string]story.Node{
			"start": {
				ID: "start", Type: "choice", Speaker: "Narrator",
				Text: "A fork in the road. A guarded gate and a dark forest await.",
				Choices: []story.Choice{
					{ID: "to_gate", Text: "Walk up to the guarded gate", Target: "gate"},
					{ID: "to_forest", Text: "Slip into the dark forest", Target: "forest"},
				},
			},
			"gate": {
				ID: "gate", Type: "choice", Speaker: "Guard",
				Text: "The toll is 10 gold. Rich travelers may pass; others can beg.",
				Choices: []story.Choice{
					{
						ID: "pay", Text: "Pay the 10 gold toll", Target: "city",
						Condition: fact("gold", ">=", float64(10)),
						Effects:   []story.Effect{{Op: "inc", Key: "gold", Value: float64(-10)}, {Op: "flag", Key: "enteredCity"}},
					},
					{ID: "beg", Text: "Beg the guard for mercy", Target: "gate_rejected"},
				},
			},
			"gate_rejected": {
				ID: "gate_rejected", Type: "choice", Speaker: "Guard",
				Text: "No coin, no entry. The forest path is still open.",
				Choices: []story.Choice{
					{ID: "back_forest", Text: "Head to the forest", Target: "forest"},
					{ID: "wait", Text: "Wait and think", Target: "start"},
				},
			},
			"forest": {
				ID: "forest", Type: "choice", Speaker: "Stranger",
				Text: "A stranger offers a pouch of gold, but only once.",
				Choices: []story.Choice{
					{
						ID: "take_gold", Text: "Take the pouch (+10 gold)", Target: "forest_after",
						Condition: fact("tookGold", "missing", nil),
						Effects:   []story.Effect{{Op: "inc", Key: "gold", Value: float64(10)}, {Op: "flag", Key: "tookGold"}},
					},
					{ID: "decline", Text: "Refuse and move on", Target: "forest_after"},
				},
			},
			"forest_after": {
				ID: "forest_after", Type: "choice", Speaker: "Narrator",
				Text: "The forest path loops back toward the gate.",
				Choices: []story.Choice{
					{ID: "back_gate", Text: "Return to the gate", Target: "gate"},
					{ID: "camp", Text: "Make camp", Target: "end"},
				},
			},
			"city": {
				ID: "city", Type: "end", Speaker: "Narrator",
				Text: "You enter the city. The road continues another day.",
			},
			"end": {
				ID: "end", Type: "end", Speaker: "Narrator",
				Text: "You rest under the trees.",
			},
		},
	}
}

// HeroV2 changes the toll to 25 gold. Old saves keep playing against v1.
func HeroV2() *story.Definition {
	v2 := HeroV1()
	v2.Version = "2"
	gate := v2.Nodes["gate"]
	for i, ch := range gate.Choices {
		if ch.ID == "pay" {
			ch.Condition = fact("gold", ">=", float64(25))
			ch.Text = "Pay the 25 gold toll"
			ch.Effects = []story.Effect{{Op: "inc", Key: "gold", Value: float64(-25)}, {Op: "flag", Key: "enteredCity"}}
			gate.Choices[i] = ch
		}
	}
	v2.Nodes["gate"] = gate
	return v2
}

// All returns every built-in definition, newest last.
func All() []*story.Definition {
	return []*story.Definition{HeroV1(), HeroV2()}
}
