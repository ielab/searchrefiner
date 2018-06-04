package main

import (
	"github.com/hscells/cqr"
	"github.com/hscells/groove"
	"github.com/hscells/groove/combinator"
	"github.com/hscells/groove/stats"
	"log"
	"strconv"
)

func buildAdjTree(query cqr.CommonQueryRepresentation, id, parent, level int, ss *stats.ElasticsearchStatisticsSource) (nid int, t tree) {
	var docs int
	if documents, err := seen.Get(query); err == nil {
		docs = len(documents)
	} else {
		d, err := ss.ExecuteFast(groove.NewPipelineQuery("adj", "0", query), ss.SearchOptions())
		if err != nil {
			log.Println("something bad happened")
			log.Fatalln(err)
			panic(err)
		}
		combDocs := make(combinator.Documents, len(d))
		for i, doc := range d {
			combDocs[i] = combinator.Document(doc)
		}

		// Cache results for this query.
		seen.Set(query, combDocs)

		docs = len(d)
	}
	switch q := query.(type) {
	case cqr.Keyword:
		t.Nodes = append(t.Nodes, node{id, docs, level, q.StringPretty(), "box"})
		t.Edges = append(t.Edges, edge{parent, id, docs, strconv.Itoa(docs)})
		id++
		log.Printf("combined [atom] %v (id %v - %v docs) with parent %v at level %v\n", q.QueryString, id, docs, parent, level)
	case cqr.BooleanQuery:
		t.Nodes = append(t.Nodes, node{id, docs, level, q.StringPretty(), "circle"})
		if parent > 0 {
			t.Edges = append(t.Edges, edge{parent, id, docs, strconv.Itoa(docs)})
		}
		this := id
		id++
		for _, child := range q.Children {
			var nt tree
			id, nt = buildAdjTree(child, id, this, level+1, ss)
			t.Nodes = append(t.Nodes, nt.Nodes...)
			t.Edges = append(t.Edges, nt.Edges...)
		}
		log.Printf("combined [clause] %v (id %v - %v docs) with parent %v at level %v\n", q.Operator, id, docs, parent, level)
	}
	nid += id
	return
}

func buildTreeRec(treeNode combinator.LogicalTreeNode, id, parent, level int, ss *stats.ElasticsearchStatisticsSource) (nid int, t tree) {
	if treeNode == nil {
		log.Printf("treeNode %v was nil (top treeNode?) (id %v) with parent %v at level %v\n", treeNode, id, parent, level)
		return
	}
	docs := treeNode.Documents(seen)
	switch n := treeNode.(type) {
	case combinator.Combinator:
		t.Nodes = append(t.Nodes, node{id, len(docs), level, n.String(), "circle"})
		if parent > 0 {
			t.Edges = append(t.Edges, edge{parent, id, len(docs), strconv.Itoa(len(docs))})
		}
		this := id
		id++
		for _, child := range n.Clauses {
			if child == nil {
				log.Printf("child treeNode %v (%v; id: %v) combined with %v and level %v\n", treeNode, child, id, parent, level)
				continue
			}
			var nt tree
			id, nt = buildTreeRec(child, id, this, level+1, ss)
			t.Nodes = append(t.Nodes, nt.Nodes...)
			t.Edges = append(t.Edges, nt.Edges...)
		}
		log.Printf("combined [clause] %v (id %v - %v docs) with parent %v at level %v\n", treeNode, id, len(docs), parent, level)
	case combinator.Atom:
		t.Nodes = append(t.Nodes, node{id, len(docs), level, n.String(), "box"})
		t.Edges = append(t.Edges, edge{parent, id, len(docs), strconv.Itoa(len(docs))})
		id++
		log.Printf("combined [atom] %v (id %v - %v docs) with parent %v at level %v\n", treeNode, id, len(docs), parent, level)
	case combinator.AdjAtom:
		id, t = buildAdjTree(n.Query(), id, parent, level, ss)
		log.Printf("combined [adj] %v (id %v - %v docs) with parent %v at level %v\n", treeNode, id, len(docs), parent, level)
	}
	nid += id
	return
}

func buildTree(node combinator.LogicalTreeNode, ss *stats.ElasticsearchStatisticsSource) (t tree) {
	_, t = buildTreeRec(node, 1, 0, 0, ss)
	log.Println("finished processing query, tree has been constructed")
	return
}
