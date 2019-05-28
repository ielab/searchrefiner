package main

import (
	"fmt"
	"github.com/hscells/cqr"
	"github.com/hscells/groove/combinator"
	"github.com/hscells/groove/stats"
	"github.com/hscells/transmute/fields"
	"github.com/ielab/searchrefiner"
	"log"
	"strings"
)

type node struct {
	ID    int    `json:"id"`
	Value int    `json:"value"`
	Level int    `json:"level"`
	Label string `json:"label"`
	Title string `json:"title"`
	Shape string `json:"shape"`
}

type edge struct {
	From  int    `json:"from"`
	To    int    `json:"to"`
	Value int    `json:"value"`
	Label string `json:"label"`
}

type tree struct {
	Nodes     []node `json:"nodes"`
	Edges     []edge `json:"edges"`
	relevant  map[combinator.Document]struct{}
	NumRelRet int
	NumRel    int
}

func fmtLabel(retrieved int, relret int) string {
	return fmt.Sprintf("%v (%v)", retrieved, relret)
}

var fieldMapping = map[string]string{
	fields.Affiliation:                  "Affiliation",
	fields.AllFields:                    "All Fields",
	fields.Author:                       "Author",
	fields.Authors:                      "Authors",
	fields.AuthorCorporate:              "Author - Corporate",
	fields.AuthorFirst:                  "Author - First",
	fields.AuthorFull:                   "Author - Full",
	fields.AuthorIdentifier:             "Author - Identifier",
	fields.AuthorLast:                   "Author - Last",
	fields.Book:                         "Book",
	fields.DateCompletion:               "Date - Completion",
	fields.ConflictOfInterestStatements: "Conflict Of Interest Statements",
	fields.DateCreate:                   "Date - Create",
	fields.DateEntrez:                   "Date - Entrez",
	fields.DateMeSH:                     "Date - MeSH",
	fields.DateModification:             "Date - Modification",
	fields.DatePublication:              "Date - Publication",
	fields.ECRNNumber:                   "EC/RN Number",
	fields.Editor:                       "Editor",
	fields.Filter:                       "Filter",
	fields.GrantNumber:                  "Grant Number",
	fields.ISBN:                         "ISBN",
	fields.Investigator:                 "Investigator",
	fields.InvestigatorFull:             "Investigator - Full",
	fields.Issue:                        "Issue",
	fields.Journal:                      "Journal",
	fields.Language:                     "Language",
	fields.LocationID:                   "Location ID",
	fields.MeSHMajorTopic:               "MeSH Major Topic",
	fields.MeSHSubheading:               "MeSH Subheading",
	fields.MeSHTerms:                    "MeSH Terms",
	fields.OtherTerm:                    "Other Term",
	fields.Pagination:                   "Pagination",
	fields.PharmacologicalAction:        "Pharmacological Action",
	fields.PublicationType:              "Publication Type",
	fields.Publisher:                    "Publisher",
	fields.SecondarySourceID:            "Secondary Source ID",
	fields.SubjectPersonalName:          "Subject Personal Name",
	fields.SupplementaryConcept:         "Supplementary Concept",
	fields.FloatingMeshHeadings:         "Floating MeshHeadings",
	fields.TextWord:                     "Text Word",
	fields.Title:                        "Title",
	fields.TitleAbstract:                "Title/Abstract",
	fields.TransliteratedTitle:          "Transliterated Title",
	fields.Volume:                       "Volume",
	fields.MeshHeadings:                 "MeSH Headings",
	fields.MajorFocusMeshHeading:        "Major Focus MeSH Heading",
	fields.PublicationDate:              "Publication Date",
	fields.PublicationStatus:            "Publication Status",
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
	docs := treeNode.Documents(searchrefiner.QueryCacher)
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
		t.Nodes = append(t.Nodes, node{id, len(docs), level, n.String(), fmtLabel(len(docs), foundRel), "circle"})
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
		log.Printf("combined [clause#%d] %v (id %v - %v docs) with parent %v at level %v\n", n.Hash, treeNode, id, len(docs), parent, level)
	case combinator.Atom:
		q := n.Query().(cqr.Keyword)
		mappedFields := make([]string, len(q.Fields))
		for i, field := range q.Fields {
			mappedFields[i] = fieldMapping[field]
		}
		t.Nodes = append(t.Nodes, node{id, len(docs), level, fmt.Sprintf("%s[%s]", q.QueryString, strings.Join(mappedFields, ",")), fmtLabel(len(docs), foundRel), "box"})
		t.Edges = append(t.Edges, edge{parent, id, len(docs), fmtLabel(len(docs), foundRel)})
		id++
		fmt.Println(n.Query())
		log.Printf("combined [atom#%d] %s%s (id %v - %v docs) with parent %v at level %v\n", n.Hash, q.QueryString, q.Fields, id, len(docs), parent, level)
	}
	nid += id
	return
}

func buildTree(node combinator.LogicalTreeNode, ss stats.StatisticsSource, relevant ...combinator.Document) (t tree) {
	_, t = buildTreeRec(node, 1, 0, 0, ss, relevant...)
	log.Println("finished processing query, tree has been constructed")
	return
}
