package validate

import (
	"slices"
	"strings"
)

// graph is the supersede lineage as steps 8, 9 and 10 see it.
//
// Its nodes are the ids of every record this binary READ — not only the ones
// step 5 accepted. That distinction is the whole point of this type, and getting
// it wrong once already cost the milestone its only fatal check: a graph built
// from the step-5 survivors loses the edges of every rejected record, so a
// single unknown JSON property on one member of a supersede cycle deleted the
// cycle rather than the property. Decision D-39 makes a cycle fatal because "no
// lineage has an end and every answer about which Decision is current is wrong",
// and that is exactly as true when one member also breaks its schema.
//
// Decision D-40 is honoured by *who gets reported*, not by who is in the graph.
// A record step 5 rejected is not asked steps 6-11, so no finding is attached to
// its path — but the records beside it are still asked, and their answers have to
// be computed over the store as it actually is. askedFor is where that line is
// drawn; subjectsFor is the structural view that ignores it.
//
// Decision D-45 is a different rule and is untouched here. It is about records
// the loader could NOT read, of which this graph knows nothing at all: they have
// no ids, no bytes and no edges, so a chain through one still closes nothing and
// a lineage split by one is still two lineages.
type graph struct {
	// byID holds every subject carrying an id. It is a slice because a
	// duplicate id — step 7's condition — puts two records under one key, and
	// dropping one of them would hide a file from steps 9 and 10 that step 7 has
	// already proved is there.
	byID map[string][]*subject
	// ids is every node, sorted, so that a traversal visits them in the same
	// order on every run. The published findings do not depend on it — the
	// components below are sorted on the way out, and Check sorts again — but a
	// reproducible traversal is what makes a failure in this file debuggable
	// rather than a different shape each time it is run.
	ids []string
	// edges maps a record id to the ids it supersedes that resolve to a node,
	// deduplicated and sorted. Unresolvable targets are step 8's business and
	// are deliberately absent here.
	edges map[string][]string
}

// newGraph indexes every record the loader read.
//
// A rejected record keeps its edges, because the only fields the cross-record
// steps read out of a body are "supersedes", "id" and "status" — and a malformed
// or wrongly-typed one of those is either a decode failure, which leaves
// subject.doc zero and so draws no edge, or a step-5 finding against that record
// itself. An unrelated property being wrong says nothing about which record
// supersedes which.
//
// One exclusion is narrow and deliberate: a record carrying no id contributes no
// node. An id-less record cannot be referenced and cannot be reached, so it has
// no place in a relation keyed entirely by id — and indexing every such record
// under the empty string would fuse them into one node whose edges are the union
// of all of theirs, joining lineages that have nothing to do with each other.
// Both shipped schema documents require an id, so this is always a record step 5
// has already rejected, and it is reported there.
func newGraph(subjects []*subject) *graph {
	g := &graph{
		byID:  make(map[string][]*subject, len(subjects)),
		edges: make(map[string][]string, len(subjects)),
	}
	for _, s := range subjects {
		if s.ref.ID == "" {
			continue
		}
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
		// Sorted and compacted for a reader's benefit, not for the report's.
		// Measured (FIX-E's re-sweep, mutation W3): replacing both with
		// `g.edges[id] = targets` changes no published finding, because a
		// repeated edge is followed twice to the same conclusion and the order
		// of one node's targets is washed out by the sorts on the way out.
		// Neither line is load-bearing; both are kept so a dump of this map
		// during debugging is canonical.
		slices.Sort(targets)
		g.edges[id] = slices.Compact(targets)
	}
	return g
}

// resolves reports whether an id names a record this binary read.
//
// It does not ask whether that record is valid, and step 8 is why. The question
// step 8 puts is "does the record this id names exist", and a record that was
// read and then rejected by step 5 exists — it is named in the same report, one
// line above, by a step-5 finding telling the reader to correct it. Answering
// "there is no such record" about it would be the fabricated claim decision D-45
// forbids, arrived at from the other direction.
func (g *graph) resolves(id string) bool {
	_, ok := g.byID[id]
	return ok
}

// subjectsFor returns every record carrying an id, in the order they were read,
// whatever step 5 made of it. It is the structural view: who is in the lineage.
func (g *graph) subjectsFor(id string) []*subject { return g.byID[id] }

// askedFor returns the records carrying an id that step 5 accepted — the ones
// decision D-40 lets steps 9 and 10 report a finding against.
//
// The split between this and subjectsFor is D-40 stated as data flow. A rejected
// record shapes the graph, because the relation it declares is real; it does not
// receive a finding from a step that was never asked about it, because the
// fields those steps would judge it on are the ones step 5 has just said are
// wrong.
func (g *graph) askedFor(id string) []*subject {
	carrying := g.byID[id]
	asked := make([]*subject, 0, len(carrying))
	for _, s := range carrying {
		if s.schemaValid {
			asked = append(asked, s)
		}
	}
	return asked
}

// closedGroups returns every set of records that lie on a closed supersede walk:
// the non-trivial strongly connected components of the graph, plus every node
// carrying a self-edge. Each group is sorted, and the groups are ordered by
// their first member.
//
// Only the first of those two orderings is load-bearing, and the difference is
// worth stating because both look alike. Sorting a group's *members* is
// published: step 9 renders the member list into every finding it writes, and
// TestAClosedGroupIsNamedTheSameWayWhicheverRecordTheWalkEnteredItFrom goes red
// when it is dropped. Ordering the *groups* among themselves is not: every
// finding they produce goes through sorted(), which re-orders the whole report
// by path and step. Reversing the group order was measured to change nothing
// (FIX-E's re-sweep, mutation W1). It is kept because a function that returns
// components in an arbitrary order invites a caller to depend on one.
//
// Strong connectivity is the right decomposition and a depth-first walk's back
// edges are not, which is the second defect this file has carried. Reporting one
// cycle per distinct back edge names the nodes on the *tree path* that the back
// edge closed — so of five records that all lie on one closed walk, the two the
// traversal reached by a forward or cross edge appeared in no finding at all,
// while the comment above the step promised "every member of the cycle is
// reported". An SCC is exactly the set of records that can each be reached from
// every other, which is exactly the set for which "which record is current" has
// no answer, and it does not depend on where a walk entered.
//
// A component of one node is only closed if that node supersedes itself. Spec
// §95 step 9 counts that: "supersedes: [its own id]" is a lineage with no end
// just as surely as a longer loop.
//
// Tarjan's algorithm, in its standard form. It visits every node and every edge
// once, so a store of n records with m supersede entries costs O(n+m) — the
// previous walk re-rotated and re-keyed a slice per back edge.
func (g *graph) closedGroups() [][]string {
	var (
		index   = make(map[string]int, len(g.ids))
		lowLink = make(map[string]int, len(g.ids))
		onStack = make(map[string]bool, len(g.ids))
		stack   = make([]string, 0, len(g.ids))
		next    int
		groups  = make([][]string, 0, 1)
	)

	var visit func(string)
	visit = func(id string) {
		index[id] = next
		lowLink[id] = next
		next++
		stack = append(stack, id)
		onStack[id] = true

		for _, target := range g.edges[id] {
			switch _, seen := index[target]; {
			case !seen:
				visit(target)
				lowLink[id] = min(lowLink[id], lowLink[target])
			case onStack[target]:
				// A node still on the stack is an ancestor of this one on the
				// current walk, so the edge closes a loop through it.
				lowLink[id] = min(lowLink[id], index[target])
			}
		}

		if lowLink[id] != index[id] {
			return
		}

		component := make([]string, 0, 2)
		for {
			top := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			onStack[top] = false
			component = append(component, top)
			if top == id {
				break
			}
		}
		if len(component) == 1 && !slices.Contains(g.edges[id], id) {
			// One node with no edge back to itself is a record in nobody's loop.
			// Every node is in some component, so without this the step would
			// report the entire store.
			return
		}
		slices.Sort(component)
		groups = append(groups, component)
	}

	for _, id := range g.ids {
		if _, seen := index[id]; !seen {
			visit(id)
		}
	}

	slices.SortFunc(groups, func(a, b []string) int { return strings.Compare(a[0], b[0]) })
	return groups
}

// lineages partitions the nodes into supersede lineages: the weakly connected
// components of the graph, each sorted, and the components themselves ordered by
// their first member.
//
// Weak connectivity rather than reachability is what "one lineage" means. Two
// records that both supersede a third are in one lineage even though neither
// reaches the other, and that fork is precisely what step 10 is looking for.
//
// Neither ordering below reaches the report. Step 10 does not publish the
// partition; it builds its own list out of the *active* subjects in each
// component and sorts that, and every finding then goes through sorted(). Both
// the union's smaller-id-wins tie-break and the per-component member sort were
// measured to be unobservable (FIX-E's re-sweep, mutations W4 and W1c) — the
// input is already deterministic, because g.ids is sorted, so the partition
// cannot vary between runs either way. They are kept as a canonical form for a
// caller who reads this partition directly, not as guards; a test that claimed
// to hold them would be one no mutation could falsify.
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
