<script setup lang="ts">
import {computed,onMounted,ref} from 'vue'
import {api} from '../api'
type Book={id:string,title:string,author:string,edition:string}
type Remedy={id:string,canonical_name:string,preparation_key:string}
type Ancestor={id:string,heading:string}
type Rubric={ancestors:Ancestor[],id:string,source_id:string,parent_id:string|null,heading:string,full_path:string[],title:string,edition:string,verified_remedy_count:number,has_children:boolean}
type Location={exact_text:string,start_character:number,end_character:number,original_url:string}
type Detail={id:string,locations:Location[],remedies:{id:string,remedy_id:string,canonical_name:string,preparation_key:string,source_notation:string,grade:number|null,grade_scheme:string,locations:Location[]}[]}
const catalog=ref<{sources:Book[],chapters:{source_id:string,chapter:string}[],remedies:Remedy[]}>({sources:[],chapters:[],remedies:[]})
const scope=ref('all'),sourceIDs=ref<string[]>([]),query=ref(''),chapter=ref(''),remedy=ref('')
const parent=ref(''),trail=ref<Ancestor[]>([]),offset=ref(0),total=ref(0),items=ref<Rubric[]>([])
const loading=ref(false),error=ref(''),coverage=ref(''),detail=ref<Detail|null>(null),detailLoading=ref('')
let generation=0,detailGeneration=0
const chapters=computed(()=>[...new Set(catalog.value.chapters.filter(x=>scope.value==='all'||sourceIDs.value.includes(x.source_id)).map(x=>x.chapter))].sort())
const previousSearch=ref<{parent:string,trail:Ancestor[],offset:number,query:string,chapter:string}|null>(null)
async function search(reset=false){
 const current=++generation;detailGeneration++;detail.value=null;detailLoading.value='';loading.value=true;error.value='';items.value=[];total.value=0
 if(reset){offset.value=0;parent.value='';trail.value=[];previousSearch.value=null}
 const p=new URLSearchParams({q:query.value,scope:scope.value,chapter:chapter.value,parent:parent.value,offset:String(offset.value)})
 for(const id of sourceIDs.value)if(scope.value==='selected')p.append('source_id',id)
 if(remedy.value)p.set('remedy_id',remedy.value)
 try{const result=await api<{items:Rubric[],total:number,coverage:string}>('/repertory/rubrics?'+p);if(current===generation){items.value=result.items;total.value=result.total;coverage.value=result.coverage}}
 catch(e){if(current===generation)error.value=e instanceof Error?e.message:'Could not search rubrics.'}
 finally{if(current===generation)loading.value=false}
}
async function inspect(row:Rubric){
 if(detail.value?.id===row.id){detail.value=null;return}
 const current=++detailGeneration;detail.value=null;detailLoading.value=row.id;error.value=''
 try{const result=await api<Detail>('/repertory/rubrics/'+row.id);if(current===detailGeneration)detail.value=result}
 catch(e){if(current===detailGeneration)error.value=e instanceof Error?e.message:'Could not inspect rubric.'}
 finally{if(current===detailGeneration)detailLoading.value=''}
}
function children(row:Rubric){parent.value=row.id;trail.value=[...row.ancestors,row];offset.value=0;query.value='';search()}
function showInTree(row:Rubric){
 if(!previousSearch.value)previousSearch.value={parent:parent.value,trail:[...trail.value],offset:offset.value,query:query.value,chapter:chapter.value}
 parent.value=row.parent_id||'root';trail.value=[...row.ancestors];offset.value=0;query.value='';chapter.value=row.full_path[0];search()
}
function back(){const saved=previousSearch.value;if(!saved)return;parent.value=saved.parent;trail.value=saved.trail;offset.value=saved.offset;query.value=saved.query;chapter.value=saved.chapter;previousSearch.value=null;search()}
function up(){trail.value.pop();parent.value=trail.value.at(-1)?.id||'root';offset.value=0;search()}
function page(delta:number){offset.value+=delta;search()}
function reverse(id:string){remedy.value=id;query.value='';search(true)}
onMounted(async()=>{try{catalog.value=await api('/repertory/catalog');await search()}catch(e){error.value=e instanceof Error?e.message:'Could not load repertories.'}})
</script>

<template>
 <section class="repertory-browser">
  <h1>Repertory</h1>
  <p>Explore reviewed rubric paths and remedy memberships in your published sources.</p>
  <form class="repertory-filters" @submit.prevent="search(true)">
   <label>Search rubric wording<input v-model="query" maxlength="300" placeholder="For example: fear dark" /></label>
   <label>Source scope<select v-model="scope" @change="chapter='';search(true)"><option value="all">All eligible repertories</option><option value="selected">Selected sources only</option></select></label>
   <fieldset v-if="scope==='selected'"><legend>Sources and editions</legend>
    <label v-for="book in catalog.sources" :key="book.id"><input v-model="sourceIDs" type="checkbox" :value="book.id" @change="chapter='';search(true)" />{{book.title}} · {{book.author}} · {{book.edition||'Edition unspecified'}}</label>
    <p v-if="!sourceIDs.length">Select at least one source. An empty selection returns no rubrics.</p>
   </fieldset>
   <label>Chapter<select v-model="chapter" @change="search(true)"><option value="">All chapters</option><option v-for="value in chapters" :key="value">{{value}}</option></select></label>
   <label>Contains remedy / preparation<select v-model="remedy" @change="search(true)"><option value="">Any verified remedy</option><option v-for="value in catalog.remedies" :key="value.id" :value="value.id">{{value.canonical_name}} · {{value.preparation_key}}</option></select></label>
   <button type="submit" :disabled="loading">Search</button>
   <button type="button" :disabled="loading" @click="query='';parent='root';trail=[];offset=0;previousSearch=null;search()">Browse root rubrics</button>
  </form>
  <p v-if="error" role="alert" class="error">{{error}} <button @click="search()">Retry search</button></p>
  <p v-if="previousSearch"><button @click="back">Back to search results</button></p>
  <nav v-if="parent" aria-label="Rubric tree"><span>{{trail.map(x=>x.heading).join(' → ')||'Rubric tree'}}</span> <button v-if="trail.length" @click="up">Up one level</button></nav>
  <p role="status">{{loading?'Loading reviewed rubrics…':`${total} matching reviewed rubrics`}}</p>
  <p class="coverage">{{coverage}}</p>
  <p v-if="!loading&&!error&&!items.length">No verified rubrics match this scope. Passage availability does not establish reviewed rubric structure or membership.</p>
  <article v-for="row in items" :key="row.id" class="rubric">
   <small>{{row.title}} · {{row.edition||'Edition unspecified'}}</small>
   <h2>{{row.full_path.join(' → ')}}</h2>
   <p>{{row.verified_remedy_count}} verified {{row.verified_remedy_count===1?'remedy':'remedies'}} · list coverage not established</p>
   <div class="rubric-actions">
    <button :aria-expanded="detail?.id===row.id" @click="inspect(row)">{{detailLoading===row.id?'Loading…':detail?.id===row.id?'Hide details':'Inspect evidence'}}</button>
    <button v-if="row.has_children" @click="children(row)">Browse subrubrics</button>
    <button @click="showInTree(row)">Show in tree</button>
   </div>
   <div v-if="detail?.id===row.id" class="rubric-detail">
    <h3>Rubric evidence</h3>
    <div v-for="(loc,i) in detail.locations" :key="i"><blockquote>{{loc.exact_text}}</blockquote><a :href="loc.original_url" target="_blank" rel="noopener">Open saved original</a> · characters {{loc.start_character}}–{{loc.end_character}}</div>
    <p>Original links open the saved document or PDF page; the exact reviewed excerpts are shown here.</p>
    <p>Grades describe this source’s notation, not clinical efficacy or model confidence. Unknown grades remain unknown.</p>
    <p v-if="!detail.remedies.length">No verified remedy associations are available for this rubric.</p>
    <section v-for="association in detail.remedies" :key="association.id" class="association">
     <h3>{{association.source_notation}} — {{association.canonical_name}}</h3>
     <p>{{association.preparation_key}} · {{association.grade===null?'Grade unknown':`Grade ${association.grade} · ${association.grade_scheme}`}}</p>
     <button @click="reverse(association.remedy_id)">Find rubrics containing this remedy</button>
     <details><summary>Membership evidence</summary><div v-for="(loc,i) in association.locations" :key="i"><blockquote>{{loc.exact_text}}</blockquote><a :href="loc.original_url" target="_blank" rel="noopener">Open saved original</a> · characters {{loc.start_character}}–{{loc.end_character}}</div></details>
    </section>
   </div>
  </article>
  <div v-if="total>50" class="rubric-actions"><button :disabled="loading||offset===0" @click="page(-50)">Previous</button><span>{{offset+1}}–{{Math.min(offset+50,total)}} of {{total}}</span><button :disabled="loading||offset+50>=total" @click="page(50)">Next</button></div>
 </section>
</template>

<style scoped>
.repertory-browser{max-width:1100px;margin:2rem auto;padding:0 1rem}.repertory-filters{display:grid;grid-template-columns:repeat(auto-fit,minmax(220px,1fr));gap:1rem;align-items:end}.repertory-filters label{display:flex;flex-direction:column;gap:.4rem}.repertory-filters fieldset{grid-column:1/-1}.repertory-filters fieldset label{flex-direction:row;align-items:center}.repertory-filters input[type=checkbox]{width:auto}.rubric{border:1px solid #cbd5e1;border-radius:10px;padding:1rem;margin:1rem 0}.rubric h2{font-size:1.1rem;margin:.5rem 0}.rubric-actions{display:flex;gap:.7rem;flex-wrap:wrap;align-items:center}.rubric-detail{margin-top:1rem;border-top:1px solid #cbd5e1;padding-top:1rem}.association{border-top:1px solid #cbd5e1;margin-top:1rem;padding-top:.5rem}.association h3{font-size:1rem}blockquote{white-space:pre-wrap;overflow-wrap:anywhere;margin:.7rem 0;padding:.7rem;border-left:3px solid #64748b}.coverage,small{color:#526174}
</style>
