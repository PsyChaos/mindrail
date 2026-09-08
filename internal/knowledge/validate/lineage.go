package validate

import (
	"slices"
	"strings"
)

// graph is the supersede lineage as steps 8, 9 and 10 see it.
//
// Its nodes are the ids of records this binary read and step 5 accepted, and
// nothing else. That is decision D-45 built into the data rather than
// remembered by three separate steps: an edge can only point at a record that
// was read, so a chain through an id the loader could not read closes nothing,
// and two records whose only connection is such an id are not one lineage.
type graph struct {
	// byID holds every subject carrying an id. It is a slice because a
	// duplicate id — step 7's condition — puts two records under one key, and
	// dropping one of them would hide a file from steps 9 and 10 that step 7 has
	// already proved is there.
	byID map[string][]*subject
	// ids is every node, sorted, so that a traversal visits them in the same
	// order on every run.
	ids []string
	// edges maps a record id to the ids it supersedes that resolve to a node,
	// deduplicated and sorted. Unresolvable targets are step 8's business and
	// are deliberately absent here.
	edges map[string][]string
}

// newGraph indexes the records that passed step 5.
func newGraph(subjects []*subject) *graph {
	g := &graph{
		byID:  make(map[string][]*subject, len(subjects)),
		edges: make(map[string][]string, len(subjects)),
	}
	for _, s := range subjects {
		g.byID[s.ref.ID] = append(g.byID[s.ref.ID], s)
	}
	g.ids = make([]string, 0, len(g.byID))
	for id := range g.byID {
		g.ids = append(g.ids, id)
	}
	slices.Sort(g.ids)

	for _, id := range g.ids {
		targets := make([]string, 0, 2)
		for _, s := range g.byID[id] {
			for _, target := range s.doc.Supersedes {
				// Only resolvable targets become edges. A record that supersedes
				// something this binary never read contributes no lineage: the
				// walk cannot be continued from a node whose own supersedes
				// nobody could read.
				if _, ok := g.byID[target]; ok {
					targets = append(targets, target)
				}
			}
		}
		slices.Sort(targets)
		g.edges[id] = slices.Compact(targets)
	}
	return g
}

// resolves reports whether an id names a record this binary read and accepted.
func (g *graph) resolves(id string) bool {
	_, ok := g.byID[id]
	return ok
}

// subjectsFor returns every record carrying an id, in the order they were read.
func (g *graph) subjectsFor(id string) []*subject { return g.byID[id] }

// cycles returns every supersede cycle the graph contains, each rotated to start
// at its lexicographically smallest member.
//
// A cycle is a closed walk, so every member of one is a record that was read:
// there is no way to return to a node through an id nobody could read, because
// that id has no outgoing edges. Rotation is what makes the reported member
// order a property of the cycle rather than of whichever node the traversal
// happened to enter it from — without it the same three records would be
// reported in three different orders depending on map iteration.
//
// One cycle is reported per distinct back edge, deduplicated by its rotated
// members. Enumerating *every* elementary cycle in a graph is exponential and
// buys nothing: a reader has to break the lineage either way, and the members
// this reports are the records they have to look at.
func (g *graph) cycles() [][]string {
	const (
		unvisited = 0
		onStack   = 1
		done      = 2
	)

	state := make(map[string]int, len(g.ids))
	stack := make([]string, 0, len(g.ids))
	seen := make(map[string]struct{}, 2)
	found := make([][]string, 0, 1)

	var visit func(string)
	visit = func(id string) {
		state[id] = onStack
		stack = append(stack, id)

		for _, target := range g.edges[id] {
			switch state[target] {
			case unvisited:
				visit(target)
			case onStack:
				// A back edge: the walk from target to the top of the stack is
				// closed by this edge, and is therefore a cycle. A self-edge
				// makes it a cycle of one, which spec §95 step 9 counts —
				// "supersedes: [its own id]" is a lineage with no end just as
				// surely as a longer loop.
				cycle := rotateToSmallest(slices.Clone(stack[slices.Index(stack, target):]))
				key := strings.Join(cycle, "\x00")
				if _, already := seen[key]; !already {
					seen[key] = struct{}{}
					found = append(found, cycle)
				}
			}
		}

		stack = stack[:len(stack)-1]
		state[id] = done
	}

	for _, id := range g.ids {
		if state[id] == unvisited {
			visit(id)
		}
	}
	return found
}

// rotateToSmallest turns a cycle's members so the lexicographically smallest one
// comes first, preserving the walk order. The cycle DEC-0002 -> DEC-0003 ->
// DEC-0001 -> DEC-0002 always reports as DEC-0001, DEC-0002, DEC-0003 whichever
// node the traversal entered it from.
func rotateToSmallest(cycle []string) []string {
	if len(cycle) < 2 {
		return cycle
	}
	pivot := 0
	for i, id := range cycle {
		if id < cycle[pivot] {
			pivot = i
		}
	}
	// Built into a fresh slice rather than by appending the head onto the tail
	// in place: the tail shares cycle's backing array, and the reader of a
	// rotation done that way has to prove the write never overtakes the read.
	rotated := make([]string, 0, len(cycle))
	rotated = append(rotated, cycle[pivot:]...)
	rotated = append(rotated, cycle[:pivot]...)
	return rotated
}

// lineages partitions the nodes into supersede lineages: the weakly connected
// components of the graph, each sorted, and the components themselves ordered by
// their first member.
//
// Weak connectivity rather than reachability is what "one lineage" means. Two
// records that both supersede a third are in one lineage even though neither
// reaches the other, and that fork is precisely what step 10 is looking for.
func (g *graph) lineages() [][]string {
	parent := make(map[string]string, len(g.ids))
	for _, id := range g.ids {
		parent[id] = id
	}

	var find func(string) string
	find = func(id string) string {
		if parent[id] != id {
			parent[id] = find(parent[id])
		}
		return parent[id]
	}
	union := func(a, b string) {
		rootA, rootB := find(a), find(b)
		if rootA == rootB {
			return
		}
		// The smaller id always becomes the root, so the partition does not
		// depend on the order the pairs arrived in.
		if rootB < rootA {
			rootA, rootB = rootB, rootA
		}
		parent[rootB] = rootA
	}

	for _, id := range g.ids {
		for _, target := range g.edges[id] {
			union(id, target)
		}
	}

	grouped := make(map[string][]string, len(g.ids))
	for _, id := range g.ids {
		root := find(id)
		grouped[root] = append(grouped[root], id)
	}

	roots := make([]string, 0, len(grouped))
	for root := range grouped {
		roots = append(roots, root)
	}
	slices.Sort(roots)

	components := make([][]string, 0, len(roots))
	for _, root := range roots {
		members := grouped[root]
		slices.Sort(members)
		components = append(components, members)
	}
	return components
}
