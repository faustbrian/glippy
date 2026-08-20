package rules

import (
	"go/ast"

	"golang.org/x/tools/go/cfg"
)

const maxStateTransitionChanges = 1_000_000

// stateTransitionModel defines one finite, monotone data-flow problem over a
// shared Go control-flow graph. Clone isolates successor state, Merge joins an
// incoming state into an existing block entry, Transfer applies one CFG node,
// and Edge optionally refines one isolated successor state. Transfer or Edge
// returns false when the represented paths cannot continue.
//
// MaxChanges is a caller-derived bound based on graph size and lattice height.
// Reaching it fails closed so an unexpectedly non-monotone model cannot make a
// lint invocation consume unbounded work.
type stateTransitionModel[S any] struct {
	Initial S
	Entry *cfg.Block
	Clone func(S) S
	Merge func(*S, S) bool
	Transfer func(S, ast.Node) bool
	Edge func(S, *cfg.Block, *cfg.Block) bool
	MaxChanges int
}

type stateTransitionSnapshot[S any] struct {
	entries []S
	present []bool
}

func runStateTransitions[S any](
	graph *cfg.CFG,
	model stateTransitionModel[S],
) (stateTransitionSnapshot[S], bool) {
	if graph == nil ||
		len(graph.Blocks) == 0 ||
		model.Clone == nil ||
		model.Merge == nil ||
		model.Transfer == nil ||
		model.MaxChanges <= 0 {
		return stateTransitionSnapshot[S]{}, false
	}
	entries := make([]S, len(graph.Blocks))
	present := make([]bool, len(graph.Blocks))
	queued := make([]bool, len(graph.Blocks))
	entry := model.Entry
	if entry == nil {
		entry = graph.Blocks[0]
	}
	if entry == nil || entry.Index < 0 || int(entry.Index) >= len(entries) {
		return stateTransitionSnapshot[S]{}, false
	}
	entries[entry.Index] = model.Clone(model.Initial)
	present[entry.Index] = true
	queue := []*cfg.Block{entry}
	queued[entry.Index] = true
	changes := 1
	for len(queue) > 0 {
		block := queue[0]
		queue = queue[1:]
		if block == nil || block.Index < 0 || int(block.Index) >= len(entries) {
			return stateTransitionSnapshot[S]{}, false
		}
		queued[block.Index] = false
		if !block.Live || !present[block.Index] {
			continue
		}
		state := model.Clone(entries[block.Index])
		reachable := true
		for _, node := range block.Nodes {
			if !model.Transfer(state, node) {
				reachable = false
				break
			}
		}
		if !reachable {
			continue
		}
		for _, successor := range block.Succs {
			if successor == nil ||
				successor.Index < 0 ||
				int(successor.Index) >= len(entries) {
				return stateTransitionSnapshot[S]{}, false
			}
			successorState := state
			if model.Edge != nil {
				successorState = model.Clone(state)
				if !model.Edge(successorState, block, successor) {
					continue
				}
			}
			index := successor.Index
			changed := false
			if !present[index] {
				entries[index] = successorState
				if model.Edge == nil {
					entries[index] = model.Clone(successorState)
				}
				present[index] = true
				changed = true
			} else {
				changed = model.Merge(&entries[index], successorState)
			}
			if !changed {
				continue
			}
			changes++
			if changes > model.MaxChanges {
				return stateTransitionSnapshot[S]{}, false
			}
			if !queued[index] {
				queue = append(queue, successor)
				queued[index] = true
			}
		}
	}
	return stateTransitionSnapshot[S]{entries: entries, present: present}, true
}
