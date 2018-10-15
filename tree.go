package searchrefiner

import (
	"fmt"
	"github.com/hscells/cqr"
	"github.com/hscells/groove/combinator"
	gpipeline "github.com/hscells/groove/pipeline"
	"github.com/hscells/groove/stats"
	"log"
)

func fmtLabel(retrieved int, relret int) string {
	return fmt.Sprintf("%v (%v)", retrieved, relret)
}

func buildAdjTree(query cqr.CommonQueryRepresentation, id, parent, level int, ss stats.StatisticsSource, relevant ...combinator.Document) (nid int, t tree) {
	if t.relevant == nil {
		t.relevant = make(map[combinator.Document]struct{})
	}
	var docs int
	foundRel := 0
	if documents, err := QueryCacher.Get(query); err == nil {
		docs = len(documents)
		for _, doc := range documents {
			for _, rel := range relevant {
				if combinator.Document(doc) == rel {
					t.relevant[combinator.Document(doc)] = struct{}{}
					foundRel++
				}
			}
		}
	} else {
		d, err := stats.GetDocumentIDs(gpipeline.NewQuery("adj", "0", query), ss)
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
		QueryCacher.Set(query, combDocs)
		docs = len(d)

		for _, doc := range d {
			for _, rel := range relevant {
				if combinator.Document(doc) == rel {
					t.relevant[combinator.Document(doc)] = struct{}{}
					foundRel++
				}
			}
		}
	}
	switch q := query.(type) {
	case cqr.Keyword:
		t.Nodes = append(t.Nodes, node{id, docs, level, q.StringPretty(), "box"})
		t.Edges = append(t.Edges, edge{parent, id, docs, fmtLabel(docs, foundRel)})
		id++
		log.Printf("combined [atom] %v (id %v - %v docs) with parent %v at level %v\n", q.QueryString, id, docs, parent, level)
	case cqr.BooleanQuery:
		t.Nodes = append(t.Nodes, node{id, docs, level, q.StringPretty(), "circle"})
		if parent > 0 {
			t.Edges = append(t.Edges, edge{parent, id, docs, fmtLabel(docs, foundRel)})
		}
		this := id
		id++
		for _, child := range q.Children {
			var nt tree
			id, nt = buildAdjTree(child, id, this, level+1, ss, relevant...)
			t.Nodes = append(t.Nodes, nt.Nodes...)
			t.Edges = append(t.Edges, nt.Edges...)
		}
		log.Printf("combined [clause] %v (id %v - %v docs) with parent %v at level %v\n", q.Operator, id, docs, parent, level)
	}
	nid += id
	return
}

func buildTreeRec(treeNode combinator.LogicalTreeNode, id, parent, level int, ss stats.StatisticsSource, relevant ...combinator.Document) (nid int, t tree) {
	if t.relevant == nil {
		t.relevant = make(map[combinator.Document]struct{})
	}
	if treeNode == nil {
		log.Printf("treeNode %v was nil (top treeNode?) (id %v) with parent %v at level %v\n", treeNode, id, parent, level)
		return
	}
	foundRel := 0
	docs := treeNode.Documents(QueryCacher)
	for _, doc := range docs {
		for _, rel := range relevant {
			if doc == rel {
				t.relevant[doc] = struct{}{}
				foundRel++
			}
		}
	}
	switch n := treeNode.(type) {
	case combinator.Combinator:
		t.Nodes = append(t.Nodes, node{id, len(docs), level, n.String(), "circle"})
		if parent > 0 {
			t.Edges = append(t.Edges, edge{parent, id, len(docs), fmtLabel(len(docs), foundRel)})
		}
		this := id
		id++
		for _, child := range n.Clauses {
			if child == nil {
				log.Printf("child treeNode %v (%v; id: %v) combined with %v and level %v\n", treeNode, child, id, parent, level)
				continue
			}
			var nt tree
			id, nt = buildTreeRec(child, id, this, level+1, ss, relevant...)
			t.Nodes = append(t.Nodes, nt.Nodes...)
			t.Edges = append(t.Edges, nt.Edges...)
		}
		log.Printf("combined [clause] %v (id %v - %v docs) with parent %v at level %v\n", treeNode, id, len(docs), parent, level)
	case combinator.Atom:
		t.Nodes = append(t.Nodes, node{id, len(docs), level, n.String(), "box"})
		t.Edges = append(t.Edges, edge{parent, id, len(docs), fmtLabel(len(docs), foundRel)})
		id++
		log.Printf("combined [atom] %v (id %v - %v docs) with parent %v at level %v\n", treeNode, id, len(docs), parent, level)
	case combinator.AdjAtom:
		id, t = buildAdjTree(n.Query(), id, parent, level, ss, relevant...)
		log.Printf("combined [adj] %v (id %v - %v docs) with parent %v at level %v\n", treeNode, id, len(docs), parent, level)
	}
	nid += id
	return
}

func buildTree(node combinator.LogicalTreeNode, ss stats.StatisticsSource, relevant ...combinator.Document) (t tree) {
	_, t = buildTreeRec(node, 1, 0, 0, ss, relevant...)
	log.Println("finished processing query, tree has been constructed")
	return
}
