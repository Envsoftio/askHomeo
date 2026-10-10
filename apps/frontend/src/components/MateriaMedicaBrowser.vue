<script setup lang="ts">
import {computed,onMounted,onUnmounted,ref} from 'vue'
import {api} from '../api'
type Preparation={ready:number,review:number,processing:number,attention:number}
const props=defineProps<{initialRemedy?:string,preparation:Preparation}>()
const emit=defineEmits<{sources:[],research:[sourceId:string,remedyName:string]}>()
type Book={id:string,title:string,author:string,edition:string,repository:string}
type Entry={id:string,source_id:string,heading:string,canonical_name:string,preparation_key:string,title:string,author:string,edition:string,repository:string,document_format:string,location_count:number}
type Detail={id:string,locations:{exact_text:string,label:string,position:number,kind:string,original_url:string}[]}
const catalog=ref<{sources:Book[],remedies:{id:string,canonical_name:string,preparation_key:string}[]}>({sources:[],remedies:[]})
const query=ref(''),scope=ref('all'),selected=ref<string[]>([]),remedy=ref(props.initialRemedy||''),provider=ref('')
const books=computed(()=>catalog.value.sources.filter(s=>!provider.value||s.repository===provider.value))
const providers=computed(()=>[...new Set(catalog.value.sources.map(s=>s.repository).filter(Boolean))].sort())
const items=ref<Entry[]>([]),total=ref(0),offset=ref(0),loading=ref(false),error=ref(''),detail=ref<Detail|null>(null),detailLoading=ref('')
let generation=0,detailGeneration=0
async function search(reset=false){
 const version=++generation;detailGeneration++;detail.value=null;detailLoading.value='';loading.value=true;error.value='';items.value=[];total.value=0
 if(reset)offset.value=0
 const ids=provider.value?books.value.filter(s=>scope.value==='all'||selected.value.includes(s.id)).map(s=>s.id):selected.value
 const p=new URLSearchParams({q:query.value,scope:provider.value?'selected':scope.value,offset:String(offset.value)})
 if(scope.value==='selected'||provider.value)for(const id of ids)p.append('source_id',id)
 if(remedy.value)p.set('remedy_id',remedy.value)
 try{const result=await api<{items:Entry[],total:number}>('/materia-medica/entries?'+p);if(version===generation){items.value=result.items;total.value=result.total}}
 catch(e){if(version===generation)error.value=e instanceof Error?e.message:'Could not load entries.'}
 finally{if(version===generation)loading.value=false}
}
async function load(){try{catalog.value=await api('/materia-medica/catalog');await search()}catch(e){error.value=e instanceof Error?e.message:'Could not load the library.'}}
async function inspect(row:Entry){
 const version=++detailGeneration
 if(detail.value?.id===row.id){detail.value=null;return}
 detail.value=null;detailLoading.value=row.id;error.value=''
 try{const result=await api<Detail>('/materia-medica/entries/'+row.id);if(version===detailGeneration)detail.value=result}
 catch(e){if(version===detailGeneration)error.value=e instanceof Error?e.message:'Could not read this entry.'}
 finally{if(version===detailGeneration)detailLoading.value=''}
}
function page(delta:number){offset.value+=delta;search()}
onMounted(load);onUnmounted(()=>{generation++;detailGeneration++})
</script>
<template>
 <section class="mm-browser">
  <p class="eyebrow">REFERENCE LIBRARY</p><h1>Materia Medica</h1>
  <p>Read verified remedy entries across your books, authors and editions. Use an entry in research to ask questions with source citations.</p>
  <p role="status">{{props.preparation.ready}} sources ready for passage research · {{props.preparation.review}} awaiting review · {{props.preparation.processing}} processing or indexing · {{props.preparation.attention}} needing attention.</p>
  <p>After intake, review extracted text and reference identities, record rights, then publish. Publication queues passage embeddings; Ask uses the source when indexing finishes. Reference entries appear once their verified mappings and compatible index are ready.</p>
  <button @click="emit('sources')">Manage books in Sources</button>
  <form @submit.prevent="search(true)">
   <label>Remedy name or source spelling<input v-model="query" maxlength="300" placeholder="Search reviewed remedy entries" /></label>
   <label>Provider / repository<select v-model="provider" @change="search(true)"><option value="">All providers</option><option v-for="value in providers" :key="value">{{value}}</option></select></label>
   <label>Book scope<select v-model="scope" @change="search(true)"><option value="all">All eligible books</option><option value="selected">Selected books only</option></select></label>
   <fieldset v-if="scope==='selected'"><legend>Books and editions</legend><label v-for="book in books" :key="book.id"><input v-model="selected" type="checkbox" :value="book.id" @change="search(true)" />{{book.title}} · {{book.author}} · {{book.edition||'Edition unspecified'}}<span v-if="book.repository"> · {{book.repository}}</span></label><p v-if="!selected.length">Select a book. An empty selection returns no entries.</p></fieldset>
   <label>Remedy / preparation<select v-model="remedy" @change="search(true)"><option value="">All verified identities</option><option v-if="remedy&&!catalog.remedies.some(r=>r.id===remedy)" :value="remedy">Selected remedy — no eligible materia medica entry</option><option v-for="value in catalog.remedies" :key="value.id" :value="value.id">{{value.canonical_name}} · {{value.preparation_key}}</option></select></label>
   <button :disabled="loading">Search entries</button>
  </form>
  <p v-if="error" role="alert" class="error">{{error}} <button @click="load">Retry</button></p>
  <p role="status">{{loading?'Loading reviewed entries…':`${total} matching reviewed entries`}}</p>
  <p>Only approved entries in eligible published sources appear here. Coverage may be partial; books and preparations are kept separate.</p>
  <p v-if="!loading&&!error&&!items.length">No verified entries match. Add PDF, HTML or TXT books in Sources, review their text and remedy identities, then publish and index them.</p>
  <article v-for="row in items" :key="row.id">
   <small>{{row.title}} · {{row.author}} · {{row.edition||'Edition unspecified'}}<span v-if="row.repository"> · {{row.repository}}</span></small>
   <h2>{{row.heading}}</h2><p>{{row.canonical_name}} · {{row.preparation_key}} · {{row.location_count}} reviewed text spans · {{row.document_format.toUpperCase()}}</p>
   <div class="actions"><button :aria-expanded="detail?.id===row.id" @click="inspect(row)">{{detailLoading===row.id?'Loading…':detail?.id===row.id?'Close entry':'Read entry'}}</button><button @click="emit('research',row.source_id,row.canonical_name)">Use in research</button></div>
   <div v-if="detail?.id===row.id">
    <p>Reviewed excerpts in source order. Original links open the saved document or PDF page.</p>
    <section v-for="(loc,i) in detail.locations" :key="i"><h3>{{loc.label||`${loc.kind} ${loc.position}`}}</h3><blockquote>{{loc.exact_text}}</blockquote><a :href="loc.original_url" target="_blank" rel="noopener">Open saved original</a></section>
   </div>
  </article>
  <div v-if="total>50" class="actions"><button :disabled="loading||offset===0" @click="page(-50)">Previous</button><span>{{offset+1}}–{{Math.min(total,offset+50)}} of {{total}}</span><button :disabled="loading||offset+50>=total" @click="page(50)">Next</button></div>
 </section>
</template>
<style scoped>
.mm-browser{max-width:1100px;margin:2rem auto;padding:0 1rem}.mm-browser form{display:grid;grid-template-columns:repeat(auto-fit,minmax(220px,1fr));gap:1rem;margin:1.5rem 0;align-items:end}.mm-browser label{display:flex;flex-direction:column;gap:.4rem}.mm-browser fieldset{grid-column:1/-1}.mm-browser fieldset label{flex-direction:row;align-items:center;flex-wrap:wrap}.mm-browser input[type=checkbox]{width:auto}.mm-browser article{border:1px solid #cbd5e1;border-radius:10px;padding:1rem;margin:1rem 0}.mm-browser h2{font-size:1.15rem}.mm-browser h3{font-size:1rem}.mm-browser blockquote{white-space:pre-wrap;overflow-wrap:anywhere;margin:1rem 0;padding:.7rem;border-left:3px solid #64748b}.mm-browser small{color:#526174}
</style>
