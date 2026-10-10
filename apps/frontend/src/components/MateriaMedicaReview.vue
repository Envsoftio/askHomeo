<script setup lang="ts">
import {ref,watch} from 'vue'
import {api} from '../api'
const props=defineProps<{sourceId:string,status:string}>()
type Unit={id:string,kind:string,position:number,label:string,text:string,text_sha256:string,ready:boolean}
type Location={text_sha256:string,page_id?:string,document_block_id?:string,start_character:number,end_character:number,exact_text:string}
type Entry={id:string,heading:string,review_status:string,remedy_id:string,locations:Location[]}
const units=ref<Unit[]>([]),entries=ref<Entry[]>([]),remedies=ref<{id:string,canonical_name:string,preparation_key:string}[]>([])
const offset=ref(0),revision=ref(''),busy=ref(false),error=ref(''),notice=ref(''),spelling=ref(''),name=ref(''),preparation=ref(''),remedy=ref(''),reason=ref(''),confirmed=ref(false),search=ref('')
const selections=ref<Location[]>([])
const ranges=new Map<string,[number,number]>()
watch([spelling,name,preparation,remedy,reason],()=>{confirmed.value=false})
async function run(fn:()=>Promise<void>){busy.value=true;error.value='';try{await fn()}catch(e){error.value=e instanceof Error?e.message:'Request failed'}finally{busy.value=false}}
async function load(){const data=await api<{revision_id:string,units:Unit[]}>(`/sources/${props.sourceId}/mm-units?offset=${offset.value}`);if(revision.value&&revision.value!==data.revision_id){selections.value=[];confirmed.value=false;notice.value='The source revision changed. Select its text again.'}revision.value=data.revision_id;units.value=data.units;ranges.clear();entries.value=(await api<Entry[]>(`/sources/${props.sourceId}/structured-entries`)).filter(e=>e.heading&&e.remedy_id)}
watch(()=>props.sourceId,()=>{offset.value=0;revision.value='';selections.value=[];entries.value=[];notice.value='';spelling.value='';remedy.value='';name.value='';preparation.value='';reason.value='';confirmed.value=false;run(load)},{immediate:true})
function select(unit:Unit,event:Event){const el=event.target as HTMLTextAreaElement;ranges.set(unit.id,[Array.from(unit.text.slice(0,el.selectionStart)).length,Array.from(unit.text.slice(0,el.selectionEnd)).length])}
function add(unit:Unit){const chars=Array.from(unit.text);let [start,end]=ranges.get(unit.id)||[0,chars.length];if(start===end){start=0;end=chars.length}if(!end)return;selections.value.push({...(unit.kind==='page'?{page_id:unit.id}:{document_block_id:unit.id}),start_character:start,end_character:end,exact_text:chars.slice(start,end).join(''),text_sha256:unit.text_sha256});confirmed.value=false}
async function save(){await api(`/sources/${props.sourceId}/mm-entries`,{method:'POST',body:JSON.stringify({revision_id:revision.value,spelling:spelling.value,remedy_id:remedy.value||null,canonical_name:name.value,preparation_key:preparation.value,rationale:reason.value,locations:selections.value})});selections.value=[];confirmed.value=false;notice.value='Entry and remedy identity approved. Its linked text becomes eligible after publication and indexing.';await load()}
async function revoke(entry:Entry){if(!reason.value.trim())throw new Error('Enter a review reason before revoking a mapping.');await api(`/structured-entries/${entry.id}/review`,{method:'POST',body:JSON.stringify({decision:'rejected',rationale:reason.value})});await load()}
</script>
<template>
<details class="step"><summary>Materia medica: remedy identity and continuation review</summary>
<p>After checking the text, select the parts belonging to one remedy, including its heading and continuation on later pages. Stop before the next remedy. Exclude running headers and unrelated text. This verifies your selections, not the completeness of the whole book.</p>
<p v-if="error" role="alert" class="error">{{error}}</p><p v-if="notice" role="status">{{notice}}</p>
<button :disabled="busy" @click="run(load)">Refresh reviewed text</button>
<div v-if="status==='review'">
<article v-for="unit in units" :key="unit.id" class="step"><h4>{{unit.kind==='page'?'PDF page':'Text block'}} {{unit.position+1}} · {{unit.label}}</h4>
<a v-if="unit.kind==='page'" :href="`/api/v1/sources/${sourceId}/pages/${unit.position}/image`" target="_blank" rel="noopener">Open original scan</a>
<a v-if="unit.kind==='block'" :href="`/api/v1/sources/${sourceId}/document/${revision}/reader`" target="_blank" rel="noopener">Open saved original</a>
<p v-if="!unit.ready">Complete this page/block’s text review before linking it.</p>
<textarea :value="unit.text" readonly rows="7" :aria-label="`Text ${unit.position+1}`" @select="select(unit,$event)" />
<button :disabled="busy||!unit.ready||selections.length>=200" @click="add(unit)">Add selected text (or whole page/block)</button></article>
<div class="actions"><button :disabled="busy||offset===0" @click="offset=Math.max(0,offset-10);run(load)">Previous 10</button><button :disabled="busy||units.length<10" @click="offset+=10;run(load)">Next 10</button></div>
<h4>Selected continuation spans: {{selections.length}}</h4>
<ol><li v-for="(span,i) in selections" :key="i"><pre style="white-space:pre-wrap;max-height:10rem;overflow:auto">{{span.exact_text}}</pre><button @click="selections.splice(i,1);confirmed=false">Remove selection</button></li></ol>
<label>Exact remedy spelling in the selected source heading<input v-model="spelling" maxlength="300" /></label>
<label>Find an existing remedy<input v-model="search" /></label><button :disabled="busy" @click="run(async()=>{remedies=await api(`/remedies?q=${encodeURIComponent(search)}`)})">Search identities</button>
<label>Remedy identity<select v-model="remedy"><option value="">Create or reuse an exact name + preparation</option><option v-for="r in remedies" :key="r.id" :value="r.id">{{r.canonical_name}} · {{r.preparation_key}}</option></select></label>
<template v-if="!remedy"><label>Canonical remedy name<input v-model="name" maxlength="300" /></label><label>Preparation identity<input v-model="preparation" maxlength="300" placeholder="Identify the substance/preparation; do not merge distinct preparations" /></label></template>
<label>Review reason<textarea v-model="reason" rows="2" maxlength="4000" /></label>
<label><input v-model="confirmed" type="checkbox" /> I checked the source heading, remedy/preparation identity and every selected continuation against the original.</label>
<button :disabled="busy||!confirmed||!selections.length||!spelling.trim()||!reason.trim()" @click="run(save)">Approve entry and identity</button>
</div>
<p v-else>Published mappings are read-only. Reprocess or reimport to correct them in a new review candidate.</p>
<h4>Saved mappings</h4><ul><li v-for="entry in entries" :key="entry.id">{{entry.heading}} · {{entry.review_status}} · {{entry.locations.length}} text spans <button v-if="status==='review'&&['accepted','corrected'].includes(entry.review_status)" :disabled="busy" @click="run(()=>revoke(entry))">Revoke mapping using review reason</button></li></ul>
</details>
</template>
