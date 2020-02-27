---
title: Tools
---
<style>
.tabcontent {
  display: none;
  padding: 6px 12px;
  border: 1px solid #ccc;
  border-top: none;
}
</style>

<div class="btn-group btn-group-block">
  <button class="btn btn-primary" onclick="openTool(event, 'formulate')">QueryFormulate</button>
  <button class="btn btn-primary" onclick="openTool(event, 'suggest')">KeywordSuggest</button>
  <button class="btn btn-primary" onclick="openTool(event, 'vis')">QueryVis</button>
  <button class="btn btn-primary" onclick="openTool(event, 'lens')">QueryLens</button>
  <button class="btn btn-primary" onclick="openTool(event, 'doc')">AutoDoc</button>
</div>

<div id="formulate" class="tabcontent active" style="display: block">
    <strong>Description:</strong>
    <br/>
    AutoFormulate is an interface for supporting the automatic formulation of search strategies using the objective method. To reduce the human workloads, unnecessary errors, and bias in formulating search strategies, a computational adaptation to the objective method which aims to approximate human intuition in constructing a search query was proposed which does not require manual human involvement, nor trail-and-error procedures, and is capable of generating a query automatically. AutoFormulate provides a direct interface to this computational adaptation of the objective method.
    <br/><br/>
    <strong>GitHub Link: </strong>
    <br/>
    <a href="https://github.com/ielab/queryformulation">https://github.com/ielab/queryformulation</a>
    <br/><br/>
    <a href="../assets/images/Pubmed-formulation.png"><img src="../assets/images/Pubmed-formulation.png" title="queryformulation" width="100%" style="display:block"></a>
</div>

<div id="suggest" class="tabcontent">
    <strong>Description:</strong>
    <br/>
    KeywordSuggest is a tool that provides keyword suggestions given an input keyword. KeywordSuggest utilises clinical concept embeddings~\cite{vandervegt2019learning}, and PubMed term frequency statistics. Keywords are ranked by similarity and merged into a single list by normalising the similarity scores from each source. 
    <br/><br/>
    <strong>GitHub Link: </strong>
    <br/>
    <a href="https://github.com/ielab/wordsuggestion">https://github.com/ielab/wordsuggestion</a>
    <br/><br/>
    <a href="../assets/images/wordsuggestions.png"><img src="../assets/images/wordsuggestions.png" title="wordsuggest" width="100%" style="display:block"></a>
</div>

<div id="vis" class="tabcontent">
    <strong>Description:</strong>
    <br/>
    QueryVis is a tool for visualising search strategies. QueryVis presents a query as a tree with information about the number of studies retrieved as well as number of seed studies retrieved by each leaf node. These layers of information on top of the hierarchical representation of a query assist information specialists by allowing them to gain a deep understanding of what a query retrieves and exactly how it retrieves it. Furthermore, the above KeywordSuggest tool is integrated into this interface to allow information specialists to quickly find keyword suggestions to any part of the query they click on.    <br/><br/>
    <strong>GitHub Link: </strong>
    <br/>
    <a href="https://github.com/ielab/searchrefiner/tree/master/plugin/queryvis">https://github.com/ielab/searchrefiner/tree/master/plugin/queryvis</a>
    <br/><br/>
    <a href="../assets/images/queryvis-wordsuggest-post-cropped.png"><img src="../assets/images/queryvis-wordsuggest-post-cropped.png" title="queryvis" width="100%" style="display:block"></a>
</div>

<div id="lens" class="tabcontent">
    <strong>Description:</strong>
    <br/>
    QueryLens is a tool that automatically generates variations for a query (by applying transformations, e.g., adding or removing keywords, and fields, rewriting Boolean operators, or exploding MeSH headings), and allows information specialists to explore these variations. The middle panel in the interface displays the number of variations, and provides statistics and evaluation results for each variation query (using seed studies). The right panel plots each query in precision-recall space. A point in this plot may be clicked on to see information about the referring query in the middle panel.    <br/><br/>
    <strong>GitHub Link: </strong>
    <br/>
    <a href="https://github.com/ielab/querylens">https://github.com/ielab/querylens</a>
    <br/><br/>
    <a href="../assets/images/querylens.png"><img src="../assets/images/querylens.png" title="querylens" width="100%" style="display:block"></a>
</div>

<div id="doc" class="tabcontent">
    <strong>Description:</strong>
    <br/>
    AutoDoc is the first tool of its kind for supporting the documentation of search strategies. AutoDoc validates both search strategies and the seven elements that should be reported. Firstly, the tool checks for spelling mistakes and logical errors in the query. Next, it validates the forms that information specialists are required to fill that cover each of the seven elements that should be reported. After the query and forms have been validated, a report is generated. The generated search strategy report can finally be copied into the relevant section of a systematic review.
    <br/><br/>
    <strong>GitHub Link: </strong>
    <br/>
    <a href="https://github.com/ielab/autodoc">https://github.com/ielab/autodoc</a>
    <br/><br/>
    <a href="../assets/images/autodoc.png"><img src="../assets/images/autodoc.png" title="autodoc" width="100%" style="display:bock"></a>
</div>


<script>
function openTool(evt, toolName) {
    var i, tabcontent, tablinks;
    tabcontent = document.getElementsByClassName("tabcontent");
    for (i = 0; i < tabcontent.length; i++) {
    tabcontent[i].style.display = "none";
    }
    tablinks = document.getElementsByClassName("btn");
    for (i = 0; i < tablinks.length; i++) {
      tablinks[i].className = tablinks[i].className.replace(" active", "");
    }
    document.getElementById(toolName).style.display = "block";
    evt.currentTarget.className += " active";
}
</script>
