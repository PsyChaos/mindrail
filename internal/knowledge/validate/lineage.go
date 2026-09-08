package validate

import (
	"cmp"
	"slices"

	"github.com/PsyChaos/mindrail/internal/knowledge/loader"
)

// node is one vertex of the supersede lineage: a kind and an id.
//
// The kind is half the key because an id is only unique inside its kind. Both
// shipped documents pin the id's spelling with a `pattern` — "^DEC-[0-9]{4,}$"
// and "^INV-[0-9]{4,}$" — so today no record that step 5 accepted can carry the
// other kind's id, and a key of the bare string would behave identically. That
// is the point: the separation would be an accident of two regular expressions
// in a document decision D-36 lets the repository's contract change, not a
// property of this graph. A record step 5 rejected is under no such constraint
// at all, and it is in this graph (see newGraph). Keying on the pair makes
// "a decision and an invariant are never one node" true by construction.
//
// It also agrees with step 8, which has always read "supersedes" as same-kind:
// expectedPath(s.ref.Kind, target) is where a target must live, so a decision
// naming an invariant's id is a dangling reference and not a resolution.
type node struct {
	kind loader.RecordKind
	id   string
}

// compareNodes orders nodes by kind then id, so every traversal below visits
// them in the same order on every run.
func compareNodes(a, b node) int {
	return cmp.Or(cmp.Compare(a.kind, b.kind), cmp.Compare(a.id, b.id))
}

// ids renders a group of nodes as the ids a message names. A group is always
// single-kind — every edge is drawn between nodes of one kind — so the kind is
// implied by the records the finding is attached to and repeating it in the
// list would say nothing.
func ids(group []node) []string {
	rendered := make([]string, 0, len(group))
	for _, n := range group {
		rendered = append(rendered, n.id)
	}
	return rendered
}

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
// D-40 draws a second line inside this type, and it took a second defect to
// find. Being in the graph is not the same as owning a node: where an accepted
// record and a rejected one carry one id, the accepted record is who that node
// is, and only its "supersedes" leaves the node. newGraph is where that is
// written down and why.
//
// Decision D-45 is a different rule and is untouched here. It is about records
// the loader could NOT read, of which this graph knows nothing at all: they have
// no ids, no bytes and no edges, so a chain through one still closes nothing and
// a lineage split by one is still two lineages.
type graph struct {
	// byNode holds every subject carrying an id. It is a slice because a
	// duplicate id — step 7's condition — puts two records under one key, and
	// dropping one of them would hide a file from steps 9 and 10 that step 7 has
	// already proved is there.
	byNode map[node][]*subject
	// nodes is every node, sorted, so that a traversal visits them in the same
	// order on every run. The published findings do not depend on it — the
	// components below are sorted on the way out, and Check sorts again — but a
	// reproducible traversal is what makes a failure in this file debuggable
	// rather than a different shape each time it is run.
	nodes []node
	// edges maps a node to the nodes it supersedes that resolve, deduplicated
	// and sorted. Unresolvable targets are step 8's business and are
	// deliberately absent here.
	edges map[node][]node
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
// no place in a relation keyed by identity — and indexing every such record
// under the empty string would fuse them into one node whose edges are the union
// of all of theirs, joining lineages that have nothing to do with each other.
// Both shipped schema documents require an id, so this is always a record step 5
// has already rejected, and it is reported there.
//
// # Which record speaks for a node
//
// The paragraph above is about *edges*. This one is about *identity*, and the
// two are not the same question — conflating them is what MR-002 got wrong
// twice, once in each direction.
//
// A record tells this graph two things: which node it IS ("id") and what that
// node supersedes ("supersedes"). The first defect dropped the second from every
// rejected record, so one unknown property deleted a fatal cycle. Restoring it
// also started trusting the first, which nothing had checked: before the change
// only step-5 survivors were nodes, and a survivor's id is vouched for by its
// schema `pattern` *and* — because D-40 asks a survivor step 7 — a survivor that
// duplicates another's id is reported. Both guarantees went with the fix, and a
// stray draft carrying a valid record's id then had its "supersedes" unioned
// into that record's node: two correct files were named in a fatal cycle finding
// whose remedy could not be performed on either of them, and the file that
// created the edge was named nowhere.
//
// So: a rejected record's claim about what it supersedes stands, and its claim
// about which node it is does not get to override an accepted record's. When
// some subject under a node satisfied its schema, only those subjects supply
// that node's edges. That is D-40's own shape — a rejected record's conditions
// wait for step 5 and surface the moment the record is readable — applied to the
// half of the graph step 5 actually vouches for.
//
// The rejected record is still indexed: resolves and subjectsFor see it, because
// the file was read and step 8 must not call it absent. Only its edges wait.
//
// When *no* subject under a node was accepted, every one of them supplies edges.
// That is the round-1 case and it must stay open: an id whose only account is a
// rejected record is still that id's only account, and deleting it is exactly
// the fail-open that turned the milestone's one fatal check off.
func newGraph(subjects []*subject) *graph {
	g := &graph{
		byNode: make(map[node][]*subject, len(subjects)),
		edges:  make(map[node][]node, len(subjects)),
	}
	for _, s := range subjects {
		if s.ref.ID == "" {
			continue
		}
		key := node{kind: s.ref.Kind, id: s.ref.ID}
		g.byNode[key] = append(g.byNode[key], s)
	}
	g.nodes = make([]node, 0, len(g.byNode))
	for key := range g.byNode {
		g.nodes = append(g.nodes, key)
	}
	slices.SortFunc(g.nodes, compareNodes)

	for _, key := range g.nodes {
		// Counted rather than taken from askedFor, which allocates the slice it
		// returns: this asks only whether one exists, once per node, and
		// TestAllocationGrowsLinearlyWithTheStore is the reason that matters.
		vouched := false
		for _, s := range g.byNode[key] {
			vouched = vouched || s.schemaValid
		}

		targets := make([]node, 0, 2)
		for _, s := range g.byNode[key] {
			if vouched && !s.schemaValid {
				// Another record under this id satisfied its schema, so that
				// record is who this node is. This one's "supersedes" is an edge
				// out of a node the pipeline has not agreed it owns.
				continue
			}
			for _, target := range s.doc.Supersedes {
				// Only resolvable targets become edges. A record that supersedes
				// something this binary never read contributes no lineage: the
				// walk cannot be continued from a node whose own supersedes
				// nobody could read.
				to := node{kind: key.kind, id: target}
				if _, ok := g.byNode[to]; ok {
					targets = append(targets, to)
				}
			}
		}
		// Sorted and compacted for a reader's benefit, not for the report's.
		// Measured (FIX-E's re-sweep, mutation W3): replacing both with
		// `g.edges[key] = targets` changes no published finding, because a
		// repeated edge is followed twice to the same conclusion and the order
		// of one node's targets is washed out by the sorts on the way out.
		// Neither line is load-bearing; both are kept so a dump of this map
		// during debugging is canonical.
		slices.SortFunc(targets, compareNodes)
		g.edges[key] = slices.Compact(targets)
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
func (g *graph) resolves(n node) bool {
	_, ok := g.byNode[n]
	return ok
}

// subjectsFor returns every record carrying an id, in the order they were read,
// whatever step 5 made of it. It is the structural view: who is in the lineage.
func (g *graph) subjectsFor(n node) []*subject { return g.byNode[n] }

// askedFor returns the records carrying an id that step 5 accepted — the ones
// decision D-40 lets steps 9 and 10 report a finding against.
//
// The split between this and subjectsFor is D-40 stated as data flow. A rejected
// record shapes the graph, because the relation it declares is real; it does not
// receive a finding from a step that was never asked about it, because the
// fields those steps would judge it on are the ones step 5 has just said are
// wrong.
func (g *graph) askedFor(n node) []*subject {
	carrying := g.byNode[n]
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
func (g *graph) closedGroups() [][]node {
	var (
		index   = make(map[node]int, len(g.nodes))
		lowLink = make(map[node]int, len(g.nodes))
		onStack = make(map[node]bool, len(g.nodes))
		stack   = make([]node, 0, len(g.nodes))
		next    int
		groups  = make([][]node, 0, 1)
	)

	var visit func(node)
	visit = func(from node) {
		index[from] = next
		lowLink[from] = next
		next++
		stack = append(stack, from)
		onStack[from] = true

		for _, target := range g.edges[from] {
			switch _, seen := index[target]; {
			case !seen:
				visit(target)
				lowLink[from] = min(lowLink[from], lowLink[target])
			case onStack[target]:
				// A node still on the stack is an ancestor of this one on the
				// current walk, so the edge closes a loop through it.
				lowLink[from] = min(lowLink[from], index[target])
			}
		}

		if lowLink[from] != index[from] {
			return
		}

		component := make([]node, 0, 2)
		for {
			top := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			onStack[top] = false
			component = append(component, top)
			if top == from {
				break
			}
		}
		if len(component) == 1 && !slices.Contains(g.edges[from], from) {
			// One node with no edge back to itself is a record in nobody's loop.
			// Every node is in some component, so without this the step would
			// report the entire store.
			return
		}
		slices.SortFunc(component, compareNodes)
		groups = append(groups, component)
	}

	for _, from := range g.nodes {
		if _, seen := index[from]; !seen {
			visit(from)
		}
	}

	slices.SortFunc(groups, func(a, b []node) int { return compareNodes(a[0], b[0]) })
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
// the union's smaller-node-wins tie-break and the per-component member sort were
// measured to be unobservable (FIX-E's re-sweep, mutations W4 and W1c) — the
// input is already deterministic, because g.nodes is sorted, so the partition
// cannot vary between runs either way. They are kept as a canonical form for a
// caller who reads this partition directly, not as guards; a test that claimed
// to hold them would be one no mutation could falsify.
func (g *graph) lineages() [][]node {
	parent := make(map[node]node, len(g.nodes))
	for _, key := range g.nodes {
		parent[key] = key
	}

	var find func(node) node
	find = func(key node) node {
		if parent[key] != key {
			parent[key] = find(parent[key])
		}
		return parent[key]
	}
	union := func(a, b node) {
		rootA, rootB := find(a), find(b)
		if rootA == rootB {
			return
		}
		// The smaller node always becomes the root, so the partition does not
		// depend on the order the pairs arrived in.
		if compareNodes(rootB, rootA) < 0 {
			rootA, rootB = rootB, rootA
		}
		parent[rootB] = rootA
	}

	for _, key := range g.nodes {
		for _, target := range g.edges[key] {
			union(key, target)
		}
	}

	grouped := make(map[node][]node, len(g.nodes))
	for _, key := range g.nodes {
		root := find(key)
		grouped[root] = append(grouped[root], key)
	}

	roots := make([]node, 0, len(grouped))
	for root := range grouped {
		roots = append(roots, root)
	}
	slices.SortFunc(roots, compareNodes)

	components := make([][]node, 0, len(roots))
	for _, root := range roots {
		members := grouped[root]
		slices.SortFunc(members, compareNodes)
		components = append(components, members)
	}
	return components
}
