<script setup lang="ts">
import {computed,ref,watch} from 'vue'
import {api} from '../api'
const props=defineProps<{sourceId:string,status:string}>()
type Unit={id:string,kind:string,position:number,label:string,text:string,text_sha256:string,ready:boolean}
type Location={text_sha256:string,page_id?:string,document_block_id?:string,start_character:number,end_character:number,exact_text:string}
type Entry={id:string,kind:string,ordinal:number,full_path:string[],review_status:string}
type Member={remedy_id:string,canonical_name:string,preparation_key:string,source_notation:string,grade:string|number,grade_scheme:string}
const units=ref<Unit[]>([]),entries=ref<Entry[]>([]),selections=ref<Location[]>([]),members=ref<Member[]>([])
const remedies=ref<{id:string,canonical_name:string,preparation_key:string}[]>([]),identitySearch=ref('')
const revision=ref(''),offset=ref(0),parent=ref(''),heading=ref(''),reason=ref(''),confirmed=ref(false),busy=ref(false),error=ref(''),notice=ref('')
const parents=computed(()=>entries.value.filter(e=>['accepted','corrected'].includes(e.review_status)))
const ranges=new Map<string,[number,number]>()
watch([parent,heading,reason,members,selections],()=>{confirmed.value=false},{deep:true,flush:'sync'})
async function run(fn:()=>Promise<void>){busy.value=true;error.value='';try{await fn()}catch(e){error.value=e instanceof Error?e.message:'Could not complete review.'}finally{busy.value=false}}
async function load(){
 const result=await api<{revision_id:string,units:Unit[]}>(`/sources/${props.sourceId}/review-units?offset=${offset.value}`)
 if(revision.value&&revision.value!==result.revision_id){selections.value=[];parent.value='';notice.value='Source revision changed. Select and review its text again.'}
 revision.value=result.revision_id;units.value=result.units;ranges.clear()
 entries.value=(await api<Entry[]>(`/sources/${props.sourceId}/structured-entries`)).filter(e=>e.kind==='repertory_rubric')
}
watch(()=>props.sourceId,()=>{offset.value=0;revision.value='';selections.value=[];entries.value=[];members.value=[];parent.value='';heading.value='';reason.value='';notice.value='';run(load)},{immediate:true})
function select(unit:Unit,event:Event){const el=event.target as HTMLTextAreaElement;ranges.set(unit.id,[Array.from(unit.text.slice(0,el.selectionStart)).length,Array.from(unit.text.slice(0,el.selectionEnd)).length])}
function add(unit:Unit){const chars=Array.from(unit.text);let [start,end]=ranges.get(unit.id)||[0,chars.length];if(start===end){start=0;end=chars.length}if(!end)return;selections.value.push({...(unit.kind==='page'?{page_id:unit.id}:{document_block_id:unit.id}),start_character:start,end_character:end,exact_text:chars.slice(start,end).join(''),text_sha256:unit.text_sha256})}
async function save(){
 const associations=members.value.map(m=>{
  const grade=String(m.grade).trim()===''?null:Number(m.grade)
  if(grade!==null&&(!Number.isInteger(grade)||grade<1||grade>32767))throw new Error('Enter a positive whole-number grade or leave it unknown.')
  return {...m,remedy_id:m.remedy_id||null,grade}
 })
 await api(`/sources/${props.sourceId}/repertory-entries`,{method:'POST',body:JSON.stringify({revision_id:revision.value,parent_id:parent.value||null,heading:heading.value,rationale:reason.value,locations:selections.value,remedies:associations})})
 selections.value=[];members.value=[];heading.value='';confirmed.value=false;notice.value='Rubric and selected memberships approved. They become available after publication and compatible indexing.';await load()
}
async function revoke(entry:Entry){if(!reason.value.trim())throw new Error('Enter a review reason first.');await api(`/structured-entries/${entry.id}/review`,{method:'POST',body:JSON.stringify({decision:'rejected',rationale:reason.value})});parent.value='';await load();notice.value='Rubric, dependent subrubrics and memberships revoked. Review replacement mappings before publishing.'}
</script>
<template>
 <details class="step repertory-review"><summary>Repertory: rubric hierarchy and remedy membership review</summary>
  <p>For PDF, HTML and TXT sources, verify text against the original first. Create the chapter/root rubric, then its child rubrics. Add only remedy identities you have verified. This is manual structure review; uploading a book does not automatically verify its rubrics or grades.</p>
  <p v-if="error" class="error" role="alert">{{error}}</p><p v-if="notice" role="status">{{notice}}</p>
  <button :disabled="busy" @click="run(load)">Refresh reviewed text</button>
  <template v-if="status==='review'">
   <article v-for="unit in units" :key="unit.id" class="step">
    <h4>{{unit.kind==='page'?'PDF page':'Text block'}} {{unit.position+1}} · {{unit.label}}</h4>
    <a :href="unit.kind==='page'?`/api/v1/sources/${sourceId}/pages/${unit.position}/image`:`/api/v1/sources/${sourceId}/document/${revision}/reader`" target="_blank" rel="noopener">Open saved original</a>
    <p v-if="!unit.ready">Complete this page/block’s text review before mapping it.</p>
    <textarea :value="unit.text" readonly rows="6" :aria-label="`Repertory text ${unit.position+1}`" @select="select(unit,$event)" />
    <button :disabled="busy||!unit.ready||!unit.text||selections.length>=200" @click="add(unit)">Add selected evidence (or whole page/block)</button>
   </article>
   <div class="actions"><button :disabled="busy||offset===0" @click="offset=Math.max(0,offset-10);run(load)">Previous 10</button><button :disabled="busy||units.length<10" @click="offset+=10;run(load)">Next 10</button></div>
   <h4>Selected supporting spans: {{selections.length}}</h4><ol><li v-for="(loc,i) in selections" :key="i"><pre>{{loc.exact_text}}</pre><button :disabled="busy" @click="selections.splice(i,1)">Remove span</button></li></ol>
   <label>Parent rubric<select v-model="parent"><option value="">New chapter / root rubric</option><option v-for="entry in parents" :key="entry.id" :value="entry.id">{{entry.full_path.join(' → ')}}</option></select></label>
   <label>Exact heading for this rubric<input v-model="heading" maxlength="500" /></label>
   <p>Include the heading, relevant remedy list and needed context. For known numeric grades, also select the source’s grading convention. Ordinary/italic/bold notation alone does not justify a numeric conversion.</p>
   <label>Find an existing remedy identity<input v-model="identitySearch" /></label><button :disabled="busy" @click="run(async()=>{remedies=await api(`/remedies?q=${encodeURIComponent(identitySearch)}`)})">Find identities</button>
   <fieldset v-for="(member,i) in members" :key="i"><legend>Verified membership {{i+1}}</legend>
    <label>Exact source abbreviation / notation<input v-model="member.source_notation" maxlength="300" /></label>
    <label>Remedy identity<select v-model="member.remedy_id"><option value="">Create or reuse name + preparation</option><option v-for="r in remedies" :key="r.id" :value="r.id">{{r.canonical_name}} · {{r.preparation_key}}</option></select></label>
    <template v-if="!member.remedy_id"><label>Canonical name<input v-model="member.canonical_name" maxlength="300" /></label><label>Preparation identity<input v-model="member.preparation_key" maxlength="300" /></label></template>
    <label>Known numeric grade (blank = unknown)<input v-model="member.grade" type="number" min="1" max="32767" step="1" /></label>
    <label v-if="String(member.grade).trim()">Exact source grading-convention quote<textarea v-model="member.grade_scheme" rows="2" maxlength="2000" /></label>
    <button :disabled="busy" @click="members.splice(i,1)">Remove membership</button>
   </fieldset>
   <button :disabled="busy||members.length>=200" @click="members.push({remedy_id:'',canonical_name:'',preparation_key:'',source_notation:'',grade:'',grade_scheme:''})">Add verified membership</button>
   <label>Review reason<textarea v-model="reason" rows="2" maxlength="4000" /></label>
   <label class="confirmation"><input v-model="confirmed" type="checkbox" /> I checked the hierarchy, every selected span, remedy/preparation identities and any grade convention against the original. Unverified memberships are omitted.</label>
   <button :disabled="busy||!confirmed||!heading.trim()||!reason.trim()||!selections.length" @click="run(save)">Approve rubric and memberships</button>
  </template>
  <p v-else>Published structure is read-only. Reprocess/reimport into a review candidate to correct it.</p>
  <h4>Saved rubrics: {{parents.length}} approved / {{entries.length}} recorded</h4>
  <ul><li v-for="entry in entries" :key="entry.id">{{entry.full_path.join(' → ')}} · {{entry.review_status}} <button v-if="status==='review'&&['accepted','corrected'].includes(entry.review_status)" :disabled="busy" @click="run(()=>revoke(entry))">Revoke rubric and descendants using review reason</button></li></ul>
 </details>
</template>
<style scoped>
.repertory-review label{display:block;margin:.8rem 0}.repertory-review input:not([type=checkbox]),.repertory-review select,.repertory-review textarea{display:block;width:100%;margin-top:.3rem}.repertory-review pre{white-space:pre-wrap;overflow:auto;max-height:12rem}.repertory-review fieldset{margin:1rem 0}.confirmation{display:flex!important;gap:.6rem;align-items:flex-start}.confirmation input{width:auto}
</style>
