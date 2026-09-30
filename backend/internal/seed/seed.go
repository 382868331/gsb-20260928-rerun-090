// Package seed ships the built-in story definition used by the platform.
package seed

import "storyplatform/internal/story"

// Story is "雾港抉择" (Mist Harbor), a short branching story that exercises:
//   - choices revealed only when numeric/flag conditions hold,
//   - rewards (gold, flags, items) granted once per choice,
//   - interruption/resume and rehearsal branches.
func Story() *story.Definition {
	gold := func(op string, n float64) *story.Condition {
		return &story.Condition{Fact: "gold", Op: op, Value: n}
	}
	charm := &story.Condition{Fact: "charm", Op: "==", Value: true}
	return &story.Definition{
		ID:      "mist-harbor",
		Title:   "雾港抉择",
		Version: "2026-09-30.v1",
		Start:   "square",
		Nodes: map[string]story.Node{
			"square": {
				ID:   "square",
				Type: "choice",
				Text: "晨雾里的石板广场。北边城门的卫兵打着哈欠，集市的叫卖声从巷口传来。",
				Choices: []story.Choice{
					{ID: "chop_wood", Text: "去伐木场干一天活", Target: "lumberyard",
						Effects: []story.Effect{{Op: "inc", Key: "gold", Value: float64(5)}}},
					{ID: "go_market", Text: "到集市转转", Target: "market"},
					{ID: "bribe_guard", Text: "塞给卫兵十枚金币开北门", Target: "north_gate",
						Condition: gold(">=", float64(10)),
						Effects: []story.Effect{
							{Op: "inc", Key: "gold", Value: float64(-10)},
							{Op: "flag", Key: "guard_pass"},
						}},
				},
			},
			"lumberyard": {
				ID:   "lumberyard",
				Type: "choice",
				Text: "伐木场堆满潮湿的松木。木匠收下你劈好的柴火，数出八枚金币。",
				Choices: []story.Choice{
					{ID: "deliver_wood", Text: "收下工钱，回到广场", Target: "square",
						Effects: []story.Effect{{Op: "inc", Key: "gold", Value: float64(8)}}},
				},
			},
			"market": {
				ID:   "market",
				Type: "choice",
				Text: "集市摊位鳞次栉比。老巫婆的摊上摆着一枚温热的铜护符，要价十枚金币。",
				Choices: []story.Choice{
					{ID: "buy_charm", Text: "买下铜护符", Target: "market",
						Condition: &story.Condition{All: []story.Condition{
							*gold(">=", float64(10)),
							{Fact: "charm", Op: "missing"},
						}},
						Effects: []story.Effect{
							{Op: "inc", Key: "gold", Value: float64(-10)},
							{Op: "flag", Key: "charm"},
							{Op: "item", Key: "铜护符"},
						}},
					{ID: "leave_market", Text: "离开集市", Target: "square"},
				},
			},
			"north_gate": {
				ID:   "north_gate",
				Type: "choice",
				Text: "北门之外是迷雾笼罩的山路。你摸了摸怀里，似乎有什么东西在微微发烫。",
				Choices: []story.Choice{
					{ID: "show_charm", Text: "向迷雾出示铜护符", Target: "ending_secret",
						Condition: charm,
						Effects: []story.Effect{
							{Op: "flag", Key: "secret_ending"},
							{Op: "item", Key: "星光印记"},
						}},
					{ID: "walk_out", Text: "直接走进山路", Target: "ending_normal"},
				},
			},
			"ending_normal": {
				ID: "ending_normal", Type: "end",
				Text: "你沿着山路远去，雾气在身后合拢。（普通结局）",
			},
			"ending_secret": {
				ID: "ending_secret", Type: "end",
				Text: "护符化作星光指引方向，你找到了雾港传说中的古道。（隐藏结局）",
			},
		},
	}
}

// InitialFacts are the facts every new play-through starts with.
func InitialFacts() map[string]interface{} {
	return map[string]interface{}{"gold": float64(0)}
}
