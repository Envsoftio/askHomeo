<script setup lang="ts">
import {computed,nextTick,onMounted,onUnmounted,ref} from 'vue'
import {api,ApiError,type Citation,type Page,type Source} from './api'

type SourceDetail=Source&{pdf_sha256:string,pages_reviewed:number,unclassified_pages:number,missing_text_pages:number,suspect_text_pages:number,text_pages_checked:number,text_pages_total:number,text_qa_status:string,text_qa_error:string,auto_blank_pages:number,triage_status:string,triage_error:string,triage_completed:number,triage_total:number,review_coverage:{front:boolean,beginning:boolean,middle:boolean,end:boolean},rights_mark:string,rights_evidence_url:string,source_url:string,pdf_origin_url:string,edition:string,publication_info:string,repository:string,rights_statement:string,retraction_notice_url:string,edition_id:string,source_record_id:string,source_asset_id:string,processing_revision_id:string,published_revision_id:string|null,rights_decision_id:string|null}
type DOIReference={id:string,doi:string,title:string,authors:string,publication_year:number,publisher:string,work_type:string,doi_url:string,pdf_available:boolean,license_url:string,metadata_provider:string,source_id:string,retraction_notice_url:string}
type ArchiveWork={identifier:string,title:string,creator:string,year:string,record_url:string}
type ArchivePDF={name:string,url:string,bytes:number,source:string}
type ArchiveItem=ArchiveWork&{publication_info:string,rights:string,license_url:string,pdfs:ArchivePDF[]}
const tab=ref<'ask'|'sources'|'review'|'activity'>('sources')
const sources=ref<Source[]>([]),active=ref<Source|null>(null),detail=ref<SourceDetail|null>(null)
const doiReferences=ref<DOIReference[]>([]),doiInput=ref('')
const doiNotice=ref(''),lastDOIID=ref('')
const archiveQuery=ref(''),archiveResults=ref<ArchiveWork[]>([]),archiveItem=ref<ArchiveItem|null>(null),archivePage=ref(1),archiveTotal=ref(0),archiveNotice=ref('')
const pages=ref<Page[]>([]),pageOffset=ref(0),scanNumber=ref(1),selectedPage=ref<Page|null>(null)
const pageNote=ref(''),pageOutcome=ref(''),showOmitted=ref(false),needsAttention=ref(false),rightsNote=ref(''),rightsDecision=ref('')
const pageNoteRequired=computed(()=>pageOutcome.value!=='blank'&&pageOutcome.value!=='book_info')
const uploadFile=ref<File|null>(null),uploadTitle=ref(''),uploadAuthor=ref(''),uploadEdition=ref(''),uploadPublication=ref(''),uploadRepository=ref(''),uploadURL=ref(''),uploadRights=ref('')
const uploadNotice=ref('')
const linkPDF=ref(''),linkTitle=ref(''),linkAuthor=ref(''),linkEdition=ref(''),linkPublication=ref(''),linkRepository=ref(''),linkSourceURL=ref(''),linkRights=ref(''),linkNotice=ref('')
const metadata=ref({title:'',author:'',edition:'',publication_info:'',repository:'',source_url:'',rights_statement:''}),printedLabel=ref('')
const sourceAccessReason=ref('')
type AnswerCitation={id:string,label:string,title:string,author:string,printed_page:string,scan_position:number}
type SearchRecord={query:string,source_scope:string,candidate_count:number}
type EvidenceRecord={rank:number,title:string,author:string,printed_page:string,scan_position:number,image_url:string,preview:string,cited:boolean}
type ResearchSection={title:string,body:string}
const question=ref(''),answer=ref(''),answerStatus=ref(''),citations=ref<AnswerCitation[]>([]),citation=ref<Citation|null>(null)
const validQuestion=computed(()=>/[\p{L}\p{N}]{3,}/u.test(question.value))
const researchMode=ref<'quick'|'deep'>('quick')
const sourceSelectionMode=ref<'all'|'selected'>('all'),selectedSourceIds=ref<string[]>([])
const searches=ref<SearchRecord[]>([])
const evidence=ref<EvidenceRecord[]>([])
const citedEvidence=computed(()=>evidence.value.filter(item=>item.cited))
const uncitedEvidence=computed(()=>evidence.value.filter(item=>!item.cited))
const sections=ref<ResearchSection[]>([])
const omittedClaims=ref(0)
type AnswerJob={id:string,question:string,research_mode:string,status:string,stage:string,attempts:number,answer_id:string|null,error_code:string|null,error_message:string|null,created_at:string,updated_at:string}
type EvaluationReport={job_id:string,question_raw:string,answer:string,answer_status:string,job_status:string,stage:string,attempts:number,owner:string,research_mode:string,selected_source_ids:string[],created_at:string,finished_at:string|null,answer_model:string,answer_revision:string,prompt_revision:string,embedding_model:string,embedding_revision:string,metrics:{search_count:number,retrieved_passages_across_searches:number,selected_candidate_count:number,cited_passage_count:number,citation_coverage:number,supported_claim_count:number,rejected_claim_count:number,top_retrieval_score:number,mean_retrieval_score:number},searches:{ordinal:number,query:string,scope:string,candidate_count:number}[],candidates:{rank:number,score:number,chunk_id:string,title:string,author:string,scan:number,printed_page:string,preview:string,cited:boolean}[],claims:{ordinal:number,claim:string,decision:string,check_method:string}[],logs:{attempt:number,stage:string,detail:Record<string,unknown>,duration_ms:number|null,created_at:string}[]}
type SourceJob={id:string,source_id:string,title:string,kind:string,status:string,stage:string,completed:number,total:number,attempts:number,error:string|null,updated_at:string}
type SavedAnswer={answer_id:string,question?:string,answer:string,status:string,citations:AnswerCitation[],searches?:SearchRecord[],evidence?:EvidenceRecord[],sections?:ResearchSection[],omitted_claim_count?:number}
type ClaimCheck={ordinal:number,claim:string,decision:string,check_method:string,supports:{label:string,excerpt:string,excerpt_start:number,excerpt_end:number}[]}
const answerJobs=ref<AnswerJob[]>([]),sourceJobs=ref<SourceJob[]>([]),activityPage=ref(1),unreadActivity=ref(0),currentJobID=ref('')
const questionJob=computed(()=>answerJobs.value.find(job=>job.id===currentJobID.value)||(question.value.trim()?answerJobs.value.find(job=>job.question===question.value.trim()):answerJobs.value[0]))
const evaluationReport=ref<EvaluationReport|null>(null)
const claimChecks=ref<ClaimCheck[]>([])
const sourceActivityGroups=computed(()=>{const groups=new Map<string,{source_id:string,title:string,jobs:SourceJob[]}>();for(const job of sourceJobs.value){const group=groups.get(job.source_id)||{source_id:job.source_id,title:job.title,jobs:[]};group.jobs.push(job);groups.set(job.source_id,group)}return [...groups.values()]})
const runningActivity=computed(()=>answerJobs.value.filter(job=>job.status==='working'||job.status==='waiting'||job.status==='retrying').length+new Set(sourceJobs.value.filter(job=>job.status==='running'||job.status==='queued').map(job=>job.source_id)).size)
const attentionActivity=computed(()=>answerJobs.value.filter(job=>job.status==='failed').length+new Set([...sourceJobs.value.filter(job=>job.status==='failed').map(job=>job.source_id),...sources.value.filter(source=>source.status==='review'&&source.rights_status!=='denied').map(source=>source.id)]).size)
const answerParagraphs=computed(()=>answer.value.split(/\n\s*\n/).filter(Boolean).map(paragraph=>paragraph.split(/(\[E(?:[1-9]|10)\])/g).filter(Boolean)))
const sectionParts=(body:string)=>body.split(/(\[E(?:[1-9]|10)\])/g).filter(Boolean)
const citationByLabel=computed(()=>Object.fromEntries(citations.value.map(c=>[`[${c.label}]`,c])))
const sourceRows=computed(()=>{
 const grouped=new Map<string,{author:string,title:string,pages:string[],count:number}>()
 for(const c of citations.value){
  const key=`${c.author}|${c.title}`
  const row=grouped.get(key)||{author:c.author,title:c.title,pages:[],count:0}
  const page=c.printed_page?`page ${c.printed_page}`:`scan ${c.scan_position}`
  if(!row.pages.includes(page))row.pages.push(page)
  row.count++
  grouped.set(key,row)
 }
 return [...grouped.values()]
})
type IndexStatus={status:string,model:string,completed:number,total:number,error:string,can_retry:boolean}
const index=ref<IndexStatus|null>(null),indexBySource=ref<Record<string,IndexStatus>>({})
const connectionError=ref(''),actionError=ref(''),busy=ref(false)
const session=ref<{name:string,role:string}|null>(null),sessionLoading=ref(true),accessToken=ref(''),signInError=ref(''),signingIn=ref(false),requiredRole=ref<''|'admin'>('')
let authVersion=0
async function signIn(){
 signingIn.value=true;signInError.value=''
 try{session.value=await api<{name:string,role:string}>('/session',{method:'POST',body:JSON.stringify({token:accessToken.value,required_role:requiredRole.value})});authVersion++;accessToken.value='';requiredRole.value='';reconcileNow()}
 catch(e){accessToken.value='';signInError.value=e instanceof Error?e.message:'Could not sign in.'}
 finally{signingIn.value=false}
}
async function signOut(){
 try{await api('/session',{method:'DELETE'})}catch{actionError.value='Could not sign out. Check the service and try again.';return}
 authVersion++
 session.value=null;requiredRole.value='';sources.value=[];answerJobs.value=[];sourceJobs.value=[];answer.value='';citation.value=null;actionError.value='';connectionError.value='';currentJobID.value=''
 window.clearTimeout(timer);window.clearTimeout(activityTimer)
}
async function switchToAdmin(){
 await signOut()
 if(!session.value)requiredRole.value='admin'
}
async function syncSession(){
 if(!session.value)return
 const version=authVersion
 try{const current=await api<{name:string,role:string}>('/session');if(version===authVersion)session.value=current}
 catch(e){if(version===authVersion&&e instanceof ApiError&&e.status===401){session.value=null;actionError.value='';connectionError.value='';window.clearTimeout(timer);window.clearTimeout(activityTimer)}}
}
const readySources=computed(()=>sources.value.filter(s=>s.status==='published'&&!s.superseded&&indexBySource.value[s.id]?.status==='ready'))
const activeCandidate=computed(()=>active.value?sources.value.find(s=>s.supersedes_source_id===active.value?.id&&s.status!=='failed'&&s.status!=='disabled'):null)
const readyCount=computed(()=>readySources.value.length)
const selectionValid=computed(()=>sourceSelectionMode.value==='all'||selectedSourceIds.value.length>0&&selectedSourceIds.value.every(id=>readySources.value.some(s=>s.id===id)))
const reviewReady=computed(()=>detail.value?.status==='review'&&!detail.value?.retraction_notice_url)
const publicationNeeds=computed(()=>{
 const d=detail.value
 if(!d)return []
 const needs:string[]=[]
	if(d.retraction_notice_url)needs.push('This paper was retracted and cannot be published as answer evidence')
 if(d.title.startsWith('Untitled PDF (verify details)')||d.author.startsWith('Unknown author (verify details)'))needs.push('Confirm the title and author in Source details')
 if(d.unclassified_pages)needs.push(`${d.unclassified_pages} scans still need a decision`)
 if(d.missing_text_pages)needs.push(`${d.missing_text_pages} scans need text repair`)
 if(d.text_qa_status!=='done')needs.push(`Wait for the automatic text check (${d.text_pages_checked} of ${d.text_pages_total} pages checked)`)
 if(d.suspect_text_pages)needs.push(`Review ${d.suspect_text_pages} text pages flagged by the automatic check`)
 if(d.rights_status!=='allowed')needs.push('Save an allowed rights decision')
 return needs
})
const canPublish=computed(()=>reviewReady.value&&publicationNeeds.value.length===0)
let timer:number|undefined,activityTimer:number|undefined
let sourcePollInFlight=false,activityPollInFlight=false

function stateText(s:Source){
	if(doiReferences.value.some(d=>d.source_id===s.id&&d.retraction_notice_url))return 'Retracted · excluded from answers'
 if(s.superseded)return 'Replaced · past citations available'
 if(s.status==='disabled')return 'Disabled · excluded from answers'
 if(s.supersedes_source_id&&s.status!=='published')return `Candidate revision · ${s.status==='review'?'needs review':s.status==='processing'?`${s.pages_read} of ${s.pages_total} pages`:s.status}`
 if(s.status==='queued'||s.status==='processing')return `Reading scanned pages · ${s.pages_read} of ${s.pages_total}`
 if(s.status==='review')return 'Needs your review'
 if(s.status==='published')return indexBySource.value[s.id]?.status==='ready'?'Ready to ask':indexBySource.value[s.id]?.status==='failed'?'Preparation stopped':'Preparing for questions'
 if(s.status==='failed')return 'Reading stopped'
 return s.status
}
function pageState(p:Page){
 if(p.page_kind==='blank')return p.review_status==='auto_checked'?'Automatically marked blank':'Omitted from answers'
 if(p.page_kind==='book_info')return 'Omitted from answers'
 if(p.page_kind==='illustration')return 'Image kept'
 if(p.page_kind==='missing_text')return 'Text needs repair'
 if(p.page_kind==='unclassified')return 'Choose an outcome'
 if(p.text_qa_status==='suspect')return 'Check suspected text'
 if(p.text_qa_status==='passed')return 'Text checked automatically'
 return p.review_status==='reviewed'?'Checked':'Needs check'
}
async function refresh(){
 try{
  sources.value=await api<Source[]>('/sources')
  doiReferences.value=await api<DOIReference[]>('/doi-references')
  const published=sources.value.filter(s=>s.status==='published')
  const statuses=await Promise.all(published.map(s=>api<IndexStatus>(`/sources/${s.id}/index-status`)))
  indexBySource.value=Object.fromEntries(published.map((s,i)=>[s.id,statuses[i]]))
  if(active.value){
   active.value=sources.value.find(s=>s.id===active.value?.id)||null
   if(active.value){
    index.value=indexBySource.value[active.value.id]||null
    const previous=detail.value?.triage_completed
    detail.value=await api<SourceDetail>(`/sources/${active.value.id}`)
    if(needsAttention.value&&previous!==undefined&&(previous!==detail.value.triage_completed||detail.value.suspect_text_pages!==pages.value.filter(p=>p.text_qa_status==='suspect').length))await loadPages()
   }
  }
  connectionError.value=''
 }catch(e){connectionError.value=e instanceof Error?e.message:'Could not connect to the service. Try again when it is running.'}
}
async function run(work:()=>Promise<void>){
 busy.value=true;actionError.value=''
 try{await work();await refresh()}
 catch(e){
  if(e instanceof ApiError&&(e.status===401||e.status===403))await syncSession()
  if(session.value)actionError.value=e instanceof ApiError&&e.status===403?'Administrator access is required for this action. You are signed in as '+session.value.role+'.':e instanceof Error?e.message:String(e)
 }
 finally{busy.value=false}
}
function importStarter(key:'nash'|'farrington'){run(async()=>{await api(`/imports/${key}`,{method:'POST'})})}
function searchArchive(page=1){if(archiveQuery.value.trim().length<3)return;archiveNotice.value='';archiveItem.value=null;run(async()=>{
 const result=await api<{works:ArchiveWork[],total:number,page:number}>(`/repositories/archive/search?q=${encodeURIComponent(archiveQuery.value.trim())}&page=${page}`)
 archiveResults.value=result.works;archiveTotal.value=result.total;archivePage.value=result.page
})}
function inspectArchive(id:string){archiveNotice.value='';run(async()=>{archiveItem.value=await api<ArchiveItem>(`/repositories/archive/items/${encodeURIComponent(id)}`);await nextTick();document.getElementById('archive-item')?.scrollIntoView({behavior:'smooth',block:'start'})})}
function importArchivePDF(file:ArchivePDF){if(!archiveItem.value)return;const item=archiveItem.value;archiveNotice.value='';run(async()=>{
 const result=await api<{source_id:string,title:string}>('/sources/import-url',{method:'POST',body:JSON.stringify({pdf_url:file.url,title:item.title,author:item.creator,publication_info:item.publication_info,repository:'Internet Archive',source_url:item.record_url,rights_statement:item.rights||item.license_url})})
 archiveNotice.value=`“${result.title}” was downloaded and queued for review.`
 await refresh()
 const imported=sources.value.find(source=>source.id===result.source_id)
 if(imported)await chooseSource(imported)
})}
function addDOI(){if(!doiInput.value.trim())return;doiNotice.value='';run(async()=>{
 const result=await api<{id:string,title:string,pdf_available:boolean,retraction_notice_url:string}>('/doi-references',{method:'POST',body:JSON.stringify({doi:doiInput.value.trim()})})
 doiInput.value='';lastDOIID.value=result.id
 doiNotice.value=result.retraction_notice_url?`Found “${result.title}”. This article was retracted and cannot be used as answer evidence. See the linked notice below.`:`Found “${result.title}” and saved its DOI reference. ${result.pdf_available?'A PDF import is available below.':'No eligible PDF import was found.'} This paper is not yet available for answers.`
})}
function importDOI(id:string){doiNotice.value='';run(async()=>{await api(`/doi-references/${id}/import`,{method:'POST'});lastDOIID.value=id;doiNotice.value='PDF import started. Open the new source below to review it before it can be used in answers.'})}
function deleteDOI(item:DOIReference){if(!window.confirm(item.source_id?'Remove this DOI reference from the list? Its imported source and DOI provenance will be kept.':'Delete this saved DOI reference?'))return;run(async()=>{await api(`/doi-references/${item.id}`,{method:'DELETE'});if(lastDOIID.value===item.id){lastDOIID.value='';doiNotice.value=''}})}
function importPDFLink(){if(!linkPDF.value.trim())return;linkNotice.value='';run(async()=>{
 const result=await api<{title:string,author:string}>('/sources/import-url',{method:'POST',body:JSON.stringify({pdf_url:linkPDF.value.trim(),title:linkTitle.value.trim(),author:linkAuthor.value.trim(),edition:linkEdition.value.trim(),publication_info:linkPublication.value.trim(),repository:linkRepository.value.trim(),source_url:linkSourceURL.value.trim(),rights_statement:linkRights.value.trim()})})
 linkNotice.value=`PDF downloaded and queued as “${result.title}” by ${result.author}. Open its source below to verify the detected details.`;linkPDF.value='';linkTitle.value='';linkAuthor.value=''
})}
function pickPDF(event:Event){uploadFile.value=(event.target as HTMLInputElement).files?.[0]||null}
function uploadPDF(){if(!uploadFile.value)return;uploadNotice.value='';run(async()=>{
 const form=new FormData();form.set('file',uploadFile.value!);form.set('title',uploadTitle.value.trim());form.set('author',uploadAuthor.value.trim());form.set('edition',uploadEdition.value.trim());form.set('publication_info',uploadPublication.value.trim());form.set('repository',uploadRepository.value.trim());form.set('source_url',uploadURL.value.trim());form.set('rights_statement',uploadRights.value.trim())
 const result=await api<{title:string,author:string}>('/sources/upload',{method:'POST',body:form});uploadNotice.value=`PDF queued as “${result.title}” by ${result.author}. Open its source below to verify the detected details.`;uploadFile.value=null;uploadTitle.value='';uploadAuthor.value=''
})}
async function chooseSource(s:Source){active.value=s;sourceAccessReason.value='';pageOffset.value=0;selectedPage.value=null;tab.value='review';await refresh();if(detail.value)metadata.value={title:detail.value.title,author:detail.value.author,edition:detail.value.edition,publication_info:detail.value.publication_info,repository:detail.value.repository,source_url:detail.value.source_url,rights_statement:detail.value.rights_statement};needsAttention.value=true;await loadPages()}
async function loadPages(){if(!active.value)return;try{pages.value=await api<Page[]>(`/sources/${active.value.id}/pages?start=${pageOffset.value}&show_omitted=${showOmitted.value}&needs_attention=${needsAttention.value}`);connectionError.value=''}catch(e){connectionError.value='Could not load these scans. Try again.'}}
function movePages(delta:number){needsAttention.value=false;pageOffset.value=Math.max(0,pageOffset.value+delta);selectedPage.value=null;loadPages()}
async function jumpToScan(){const n=Math.min(active.value?.pages_total||1,Math.max(1,Math.floor(scanNumber.value||1)));needsAttention.value=false;showOmitted.value=true;pageOffset.value=Math.floor((n-1)/30)*30;selectedPage.value=null;await loadPages();const p=pages.value.find(p=>p.scan_page_index===n-1);if(p)selectPage(p)}
function selectPage(p:Page){selectedPage.value=p;printedLabel.value=p.printed_label;pageNote.value=p.page_kind==='unclassified'||p.review_note==='Blank page; omitted from answers.'||p.review_note==='Library or book information; omitted from answers.'?'':p.review_note||'';pageOutcome.value=p.page_kind==='unclassified'?'':p.page_kind}
function saveMetadata(){if(!active.value)return;run(async()=>{await api(`/sources/${active.value!.id}/metadata`,{method:'PATCH',body:JSON.stringify(metadata.value)})})}
function savePrintedLabel(){if(!selectedPage.value)return;run(async()=>{const id=selectedPage.value!.id;await api(`/pages/${id}/label`,{method:'PATCH',body:JSON.stringify({printed_label:printedLabel.value})});await loadPages();selectedPage.value=pages.value.find(p=>p.id===id)||null})}
function reviewPage(){if(!selectedPage.value||!pageOutcome.value||(pageNoteRequired.value&&!pageNote.value.trim()))return;run(async()=>{
  const id=selectedPage.value!.id
  await api(`/pages/${id}/review`,{method:'POST',body:JSON.stringify({note:pageNoteRequired.value?pageNote.value.trim():'',outcome:pageOutcome.value})})
  await loadPages();selectedPage.value=pages.value.find(p=>p.id===id)||null;pageNote.value=''
})}
function saveRights(){if(!active.value||!rightsDecision.value||!rightsNote.value.trim())return;run(async()=>{
 await api(`/sources/${active.value!.id}/rights`,{method:'POST',body:JSON.stringify({decision:rightsDecision.value,note:rightsNote.value.trim()})})
})}
function publish(){if(!active.value)return;run(async()=>{await api(`/sources/${active.value!.id}/publish`,{method:'POST'})})}
function retryIndex(){if(!active.value)return;run(async()=>{await api(`/sources/${active.value!.id}/reindex`,{method:'POST'})})}
function reprocessSource(){if(!active.value)return;run(async()=>{const result=await api<{source_id:string}>(`/sources/${active.value!.id}/reprocess`,{method:'POST'});await refresh();const candidate=sources.value.find(s=>s.id===result.source_id);if(candidate)await chooseSource(candidate)})}
function setSourceAccess(enable:boolean){if(!active.value||sourceAccessReason.value.trim().length<8)return;run(async()=>{await api(`/sources/${active.value!.id}/${enable?'enable':'disable'}`,{method:'POST',body:JSON.stringify({reason:sourceAccessReason.value.trim()})});sourceAccessReason.value=''})}
function setSourceSelectionMode(mode:'all'|'selected'){sourceSelectionMode.value=mode;if(mode==='selected'&&!selectedSourceIds.value.length)selectedSourceIds.value=readySources.value.map(s=>s.id)}
function showAnswer(result:SavedAnswer,openAsk=true){
 answer.value=result.answer;answerStatus.value=result.status;citations.value=result.citations;searches.value=result.searches||[];evidence.value=result.evidence||[];sections.value=result.sections||[];omittedClaims.value=result.omitted_claim_count||0
 claimChecks.value=[];api<{claims:ClaimCheck[]}>(`/research/answers/${result.answer_id}/claims`).then(data=>{claimChecks.value=data.claims}).catch(()=>{})
 if(result.question)question.value=result.question
 if(openAsk){tab.value='ask';nextTick(()=>window.scrollTo({top:0,behavior:'instant'}))}
}
async function refreshActivity(){
 try{
  const result=await api<{jobs:AnswerJob[],source_jobs:SourceJob[],unread_count:number}>(`/activity?page=${activityPage.value}`)
  answerJobs.value=result.jobs;sourceJobs.value=result.source_jobs;unreadActivity.value=result.unread_count
  const current=result.jobs.find(job=>job.id===currentJobID.value)
  if(current?.status==='finished'&&current.answer_id){const saved=await api<SavedAnswer>(`/research/answers/${current.answer_id}`);currentJobID.value='';showAnswer(saved,false)}
  connectionError.value=''
 }catch(e){connectionError.value='Connection lost. Progress may be out of date. We’ll reconnect automatically.'}
}
function ask(){if(!validQuestion.value||!selectionValid.value)return;run(async()=>{
 answer.value='';answerStatus.value='';citations.value=[];citation.value=null;searches.value=[];evidence.value=[];sections.value=[];omittedClaims.value=0
 const result=await api<{job_id:string}>('/research/answer-jobs',{method:'POST',body:JSON.stringify({question:question.value,mode:researchMode.value,source_ids:sourceSelectionMode.value==='selected'?selectedSourceIds.value:[]})})
 currentJobID.value=result.job_id;await refreshActivity()
})}
function openAnswerJob(job:AnswerJob){if(!job.answer_id)return;run(async()=>{showAnswer(await api<SavedAnswer>(`/research/answers/${job.answer_id}`));await api(`/activity/${job.id}/read`,{method:'POST'});await refreshActivity()})}
function openEvaluation(job:AnswerJob){run(async()=>{evaluationReport.value=await api<EvaluationReport>(`/admin/answer-jobs/${job.id}/evaluation`)})}
function downloadEvaluation(){if(!evaluationReport.value)return;const data=new Blob([JSON.stringify(evaluationReport.value,null,2)],{type:'application/json'});const url=URL.createObjectURL(data);const link=document.createElement('a');link.href=url;link.download=`answer-evaluation-${evaluationReport.value.job_id}.json`;link.click();URL.revokeObjectURL(url)}
function retryAnswerJob(job:AnswerJob){run(async()=>{await api(`/research/answer-jobs/${job.id}/retry`,{method:'POST'});currentJobID.value=job.id;await refreshActivity()})}
function reviseAnswerJob(job:AnswerJob){question.value=job.question;sourceSelectionMode.value='all';selectedSourceIds.value=[];tab.value='ask';nextTick(()=>window.scrollTo({top:0,behavior:'instant'}))}
function openSourceJob(job:SourceJob){const source=sources.value.find(s=>s.id===job.source_id);if(source)chooseSource(source);run(async()=>{await api(`/activity/${job.id}/read`,{method:'POST'});await refreshActivity()})}
function changeActivityPage(delta:number){activityPage.value=Math.max(1,activityPage.value+delta);refreshActivity()}
function openAskTab(){tab.value='ask';activityPage.value=1;void refreshActivity()}
function openActivityTab(){tab.value='activity';activityPage.value=1;void refreshActivity()}
function showCitation(id:string){run(async()=>{citation.value=await api<Citation>('/citations/'+id)})}
async function pollSources(){
 if(sourcePollInFlight)return
 sourcePollInFlight=true
 try{await refresh()}finally{
  sourcePollInFlight=false
  timer=window.setTimeout(pollSources,connectionError.value?30000:document.hidden?30000:runningActivity.value?5000:15000)
 }
}
async function pollActivity(){
 if(activityPollInFlight)return
 activityPollInFlight=true
 try{await refreshActivity()}finally{
  activityPollInFlight=false
  activityTimer=window.setTimeout(pollActivity,connectionError.value?30000:document.hidden?30000:runningActivity.value?3000:15000)
 }
}
function reconcileNow(){
 if(!session.value||document.hidden||!navigator.onLine)return
 void syncSession()
 window.clearTimeout(timer);window.clearTimeout(activityTimer)
 if(!sourcePollInFlight)void pollSources()
 if(!activityPollInFlight)void pollActivity()
}
function onVisibilityChange(){if(!document.hidden)reconcileNow()}
function onOffline(){connectionError.value='Connection lost. Progress is saved and will refresh when you reconnect.'}
onMounted(async()=>{
 if(!navigator.onLine)onOffline()
 try{session.value=await api<{name:string,role:string}>('/session')}catch{session.value=null}
 sessionLoading.value=false
 reconcileNow()
 window.addEventListener('online',reconcileNow)
 window.addEventListener('offline',onOffline)
 window.addEventListener('focus',reconcileNow)
 document.addEventListener('visibilitychange',onVisibilityChange)
})
onUnmounted(()=>{
 window.clearTimeout(timer);window.clearTimeout(activityTimer)
 window.removeEventListener('online',reconcileNow)
 window.removeEventListener('offline',onOffline)
 window.removeEventListener('focus',reconcileNow)
 document.removeEventListener('visibilitychange',onVisibilityChange)
})
</script>

<template>
 <main class="shell">
  <section v-if="sessionLoading" class="step"><h1>Opening source research</h1><p>Checking your session…</p></section>
  <form v-else-if="!session" class="step" @submit.prevent="signIn"><h1>{{requiredRole==='admin'?'Sign in as administrator':'Sign in'}}</h1><p v-if="requiredRole==='admin'">Enter <code>API_TOKEN</code> from the local <code>.env</code> file. <code>REVIEWER_TOKEN</code> cannot open the source catalog search.</p><p v-else>Enter your administrator or reviewer access token from the local <code>.env</code> file. Use <code>API_TOKEN</code> for administrator access and <code>REVIEWER_TOKEN</code> for reviewer access. Your work and Activity are recorded under your name.</p><label>Access token <input v-model="accessToken" type="password" autocomplete="current-password" required /></label><button :disabled="signingIn||!accessToken.trim()">{{signingIn?'Signing in…':'Sign in'}}</button><p v-if="signInError" class="error" role="alert">{{signInError}}</p></form>
  <template v-else>
  <header><strong>Source research</strong><nav aria-label="Main"><button :class="{selected:tab==='ask'}" @click="openAskTab">Ask</button><button :class="{selected:tab==='sources'}" @click="tab='sources'">Sources</button><button :class="{selected:tab==='review'}" @click="tab='review'">Review</button><button :class="{selected:tab==='activity'}" @click="openActivityTab">Activity <span v-if="runningActivity||attentionActivity||unreadActivity">({{runningActivity}} working · {{attentionActivity}} need attention<span v-if="unreadActivity"> · {{unreadActivity}} new</span>)</span></button><span>{{session.name}} ({{session.role}})</span><button @click="signOut">Sign out</button></nav></header>
  <p v-if="connectionError" class="error" role="alert">{{connectionError}} <button @click="refresh">Try again</button></p>
  <p v-if="actionError" class="error" role="alert">{{actionError}} <button @click="actionError=''">Dismiss</button></p>

  <section v-if="tab==='sources'">
   <div class="sectionhead"><div><h1>Your sources</h1><p>Add a PDF from your collection or use a starter book. Each source needs page checks, a rights decision, and passage preparation.</p></div><div v-if="session.role==='admin'" class="actions"><button :disabled="busy" @click="importStarter('nash')">Add Nash</button><button :disabled="busy" @click="importStarter('farrington')">Add Farrington</button></div></div>
   <form v-if="session.role==='admin'" class="step" @submit.prevent="searchArchive(1)"><h2>Search source catalog</h2><p>Search Internet Archive by title or author. Inspect its catalogue record and PDF before downloading. Every imported book still needs page and rights review.</p><label>Book title or author <input v-model="archiveQuery" minlength="3" maxlength="120" placeholder="Boericke or Organon of Medicine" /></label><button :disabled="busy||archiveQuery.trim().length<3">Search books</button></form>
   <div v-else class="step"><h2>Search source catalog</h2><p>Book search requires administrator access. You are signed in as {{session.name}} ({{session.role}}). Sign in with <code>API_TOKEN</code> from <code>.env</code> to search and add sources.</p><button @click="switchToAdmin">Switch to administrator</button></div>
   <div v-if="session.role==='admin'&&(archiveResults.length||archiveTotal)" class="step"><h3>Internet Archive results</h3><p>{{archiveTotal}} matching records. Choose a record to see its available PDFs.</p><div class="archive-results"><article v-for="work in archiveResults" :key="work.identifier"><strong>{{work.title||work.identifier}}</strong><p>{{work.creator||'Author not listed'}} · {{work.year||'Year unknown'}}</p><p><a :href="work.record_url" target="_blank" rel="noreferrer">Open catalogue record</a> <button :disabled="busy" @click="inspectArchive(work.identifier)">View PDFs</button></p></article></div><div class="actions"><button :disabled="busy||archivePage<=1" @click="searchArchive(archivePage-1)">Previous</button><span>Page {{archivePage}}</span><button :disabled="busy||archivePage*12>=archiveTotal" @click="searchArchive(archivePage+1)">Next</button></div></div>
   <div v-if="session.role==='admin'&&archiveItem" id="archive-item" class="step"><h3>{{archiveItem.title||archiveItem.identifier}}</h3><p>{{archiveItem.creator||'Author not listed'}} · {{archiveItem.publication_info||archiveItem.year||'Date unknown'}}</p><p><a :href="archiveItem.record_url" target="_blank" rel="noreferrer">Review catalogue record</a></p><p>Recorded rights: {{archiveItem.rights||'No rights statement supplied; check the catalogue record before publishing.'}} <a v-if="archiveItem.license_url" :href="archiveItem.license_url" target="_blank" rel="noreferrer">Licence</a></p><p v-if="!archiveItem.pdfs.length">No public PDF under 250 MB was listed for this item. Try another record or use a permitted PDF link.</p><div v-for="file in archiveItem.pdfs" :key="file.url" class="archive-file"><span>{{file.name}} · {{(file.bytes/1048576).toFixed(1)}} MB · {{file.source||'PDF'}}</span><button :disabled="busy" @click="importArchivePDF(file)">Download for review</button></div></div>
   <p v-if="archiveNotice" class="good" role="status">{{archiveNotice}}</p>
   <form v-if="session.role==='admin'" class="step" @submit.prevent="addDOI"><h2>Add by DOI</h2><p>Look up a paper and save its reference. The result appears below. It can be cited in answers only after readable text is imported, checked, and published.</p><label>DOI or doi.org link <input v-model="doiInput" placeholder="10.1186/s13643-023-02313-2" required /></label><button :disabled="busy||!doiInput.trim()">{{busy?'Looking up DOI…':'Look up DOI'}}</button><p v-if="doiNotice" class="good" role="status">{{doiNotice}}</p></form>
   <div v-if="doiReferences.length" class="step"><h2>DOI references</h2><article v-for="item in doiReferences" :key="item.id" class="doi-reference" :class="{highlighted:item.id===lastDOIID}"><strong>{{item.title}}</strong><p>{{item.authors}} · {{item.publication_year||'Year unknown'}} · {{item.publisher}}</p><p><a :href="item.doi_url" target="_blank" rel="noreferrer">{{item.doi}}</a> · {{item.metadata_provider}} metadata</p><p v-if="item.retraction_notice_url" class="error">Retracted article. Excluded from answers. <a :href="item.retraction_notice_url" target="_blank" rel="noreferrer">Read publisher notice</a>.</p><p v-else-if="item.source_id" class="good">PDF imported. Check the source below before it becomes available for answers.</p><template v-else-if="!item.retraction_notice_url"><p>Reference saved. This paper is not yet used in answers.</p><button v-if="session.role==='admin'&&item.pdf_available" :disabled="busy" @click="importDOI(item.id)">Import eligible PDF for analysis</button><p v-else>No eligible direct PDF link was found; use the PDF upload below if you have a permitted copy.</p></template><p v-if="item.license_url&&item.pdf_available"><a :href="item.license_url" target="_blank" rel="noreferrer">Check recorded licence before publishing</a></p><p><button v-if="session.role==='admin'" :disabled="busy" @click="deleteDOI(item)">Delete reference</button></p></article></div>
   <form v-if="session.role==='admin'" class="step" @submit.prevent="importPDFLink"><h2>Import from a PDF link</h2><p>Paste a direct public HTTPS PDF link. The app reads its title page and fills source details when possible. You can correct them in Review. Use one link per volume.</p><label>PDF link <input v-model="linkPDF" type="url" placeholder="https://iiif.wellcomecollection.org/pdf/…" required /></label><label>Title (optional) <input v-model="linkTitle" maxlength="300" /></label><label>Author (optional) <input v-model="linkAuthor" maxlength="300" /></label><label>Edition or volume (optional) <input v-model="linkEdition" maxlength="300" /></label><label>Publication details (optional) <input v-model="linkPublication" maxlength="500" /></label><label>Repository or collection (optional) <input v-model="linkRepository" maxlength="300" /></label><label>Catalogue or source record URL (optional) <input v-model="linkSourceURL" type="url" maxlength="1000" /></label><label>Rights statement <textarea v-model="linkRights" rows="2" maxlength="2000" placeholder="Record repository rights; review before publishing" /></label><button :disabled="busy||!linkPDF.trim()">Download and read pages</button><p v-if="linkNotice" class="good" role="status">{{linkNotice}}</p></form>
   <form v-if="session.role==='admin'" class="step" @submit.prevent="uploadPDF"><h2>Upload a PDF</h2><p>Choose a PDF; the app reads its title page to fill available details. You can correct them in Review.</p><label>PDF file <input type="file" accept="application/pdf,.pdf" @change="pickPDF" /></label><label>Title (optional) <input v-model="uploadTitle" maxlength="300" /></label><label>Author (optional) <input v-model="uploadAuthor" maxlength="300" /></label><label>Edition (optional) <input v-model="uploadEdition" maxlength="300" /></label><label>Publication details (optional) <input v-model="uploadPublication" maxlength="500" /></label><label>Repository or collection (optional) <input v-model="uploadRepository" maxlength="300" /></label><label>Source URL (optional) <input v-model="uploadURL" type="url" maxlength="1000" /></label><label>Rights statement <textarea v-model="uploadRights" rows="2" maxlength="2000" placeholder="Record the stated rights; review permission before publishing" /></label><button :disabled="busy||!uploadFile">Upload and read pages</button><p v-if="uploadNotice" class="good" role="status">{{uploadNotice}}</p></form>
   <p v-if="!sources.length" class="empty">No sources yet. Upload a PDF or add a starter book to begin.</p>
   <button v-for="s in sources" :key="s.id" class="source" @click="chooseSource(s)"><span><b>{{s.title}}</b><small>{{s.author}}</small></span><span class="state">{{stateText(s)}}</span></button>
   <p v-if="sources.some(s=>s.status==='review'&&!doiReferences.some(d=>d.source_id===s.id&&d.retraction_notice_url))" class="next">Next: open a source that needs review, compare its pages with the scans, then check its rights statement.</p>
  </section>

  <section v-else-if="tab==='review'">
   <h1>Check a source</h1>
   <p v-if="!active">Choose a book in Sources to see what needs checking.</p>
   <template v-else>
   <h2>{{active.title}}</h2>
   <div v-if="session.role==='admin'&&(active.status==='published'||active.status==='disabled')" class="step"><h3>Source availability</h3><p v-if="active.status==='disabled'">This source is excluded from new answers and its citations are unavailable until restored.</p><p v-else>Disabling a source removes it from new answers and citation access immediately. You can restore it after review.</p><label>Reason <textarea v-model="sourceAccessReason" rows="2" minlength="8" maxlength="1000" placeholder="Record why availability is changing" /></label><button :disabled="busy||sourceAccessReason.trim().length<8" @click="setSourceAccess(active.status==='disabled')">{{active.status==='disabled'?'Restore source':'Disable source'}}</button></div>
	<p v-if="detail?.retraction_notice_url" class="error" role="alert">This article was retracted. Keep it for a record of the publication, but do not use it as evidence for clinical answers. <a :href="detail.retraction_notice_url" target="_blank" rel="noreferrer">Read the publisher's retraction notice</a>.</p>
    <ol class="journey" aria-label="Source steps"><li class="done">Add source</li><li :class="{done:active.pages_read===active.pages_total}">Read pages</li><li :class="{current:active.status==='review',done:active.status==='published'}">Check pages</li><li :class="{current:canPublish,done:active.status==='published'}">Make available</li><li :class="{current:active.status==='published'&&index?.status!=='ready',done:index?.status==='ready'}">Prepare passages</li><li :class="{done:index?.status==='ready'}">Ask</li></ol>
    <p v-if="active.status==='queued'||active.status==='processing'">Reading scanned pages: {{active.pages_read}} of {{active.pages_total}}. You can leave this screen and return.</p>
    <p v-else-if="active.status==='failed'" class="error">We couldn’t finish reading this book. Your original PDF is saved. {{active.error}}</p>
    <p v-else-if="active.status==='review'&&!detail?.retraction_notice_url">The system checks the text against the scans. Only uncertain pages need your decision.</p>
    <div v-else-if="active.status==='published'" class="step"><p v-if="index?.status==='ready'" class="good">Ready to ask. {{index.completed}} of {{index.total}} passages prepared with {{index.model}}. <button @click="openAskTab">Ask about it</button></p><p v-else-if="index?.status==='pending'||index?.status==='running'">Preparing passages for search: {{index.completed}} of {{index.total}}. This continues in the background.</p><p v-else-if="index?.status==='failed'" class="error">Preparation stopped after {{index.completed}} of {{index.total}} passages: {{index.error}} <button v-if="session.role==='admin'" @click="retryIndex">Try again</button></p><p v-else>Passages have not been prepared. <button v-if="session.role==='admin'" @click="retryIndex">Prepare now</button></p></div>

    <div v-if="detail?.supersedes_source_id" class="step"><h3>Candidate revision</h3><p>This copy is being processed from the saved PDF. The current publication remains available for questions until this copy passes review and its passages are READY.</p><button v-if="sources.find(source=>source.id===detail?.supersedes_source_id)" @click="chooseSource(sources.find(source=>source.id===detail?.supersedes_source_id)!)">View current source</button></div>
    <div v-if="active.status==='published'" class="step"><h3>Processing revision</h3><p v-if="detail?.superseded">This source has been replaced for new searches. Its saved citations still open the original pages.</p><p v-else-if="activeCandidate">A candidate revision is in progress. The current source stays available until the candidate is ready.</p><p v-else>Create a new candidate from this saved PDF when page extraction or metadata needs a fresh review. The current answer evidence stays available while it runs.</p><button v-if="session.role==='admin'&&!detail?.superseded&&!activeCandidate" :disabled="busy" @click="reprocessSource">Reprocess saved PDF</button></div>
    <details v-if="detail?.processing_revision_id" class="step"><summary>Source provenance</summary><p>Edition {{detail.edition_id}}<br />Acquisition record {{detail.source_record_id}}<br />PDF asset {{detail.source_asset_id}}<br />Processing revision {{detail.processing_revision_id}}<br /><template v-if="detail.rights_decision_id">Rights decision {{detail.rights_decision_id}}<br /></template>PDF SHA-256 {{detail.pdf_sha256}}</p></details>
    <template v-if="reviewReady">
     <form v-if="session.role==='admin'" class="step" @submit.prevent="saveMetadata"><h3>Source details</h3><label>Title <input v-model="metadata.title" required /></label><label>Author <input v-model="metadata.author" required /></label><label>Edition <input v-model="metadata.edition" /></label><label>Publication details <input v-model="metadata.publication_info" /></label><label>Repository or collection <input v-model="metadata.repository" /></label><label>Source URL <input v-model="metadata.source_url" type="url" /></label><label>Rights statement <textarea v-model="metadata.rights_statement" rows="2" /></label><button :disabled="busy||!metadata.title.trim()||!metadata.author.trim()">Save details</button></form>
     <div class="step"><h3>1. Check the pages</h3><p>Clear blank scans are omitted from answers. The system compares stored text with a fresh OCR pass; pages with suspected problems stay in your review list. Every original scan remains in the PDF.</p><p v-if="detail?.triage_status==='queued'||detail?.triage_status==='running'">Checking scans without text: {{detail?.triage_completed}} of {{detail?.triage_total}}.</p><p v-if="detail?.triage_status==='failed'" class="error">Blank scan check stopped: {{detail?.triage_error}}</p><p v-if="detail?.text_qa_status==='queued'||detail?.text_qa_status==='running'">Checking text: {{detail?.text_pages_checked}} of {{detail?.text_pages_total}} pages. You can leave and return.</p><p v-if="detail?.text_qa_status==='failed'" class="error">Text check stopped: {{detail?.text_qa_error}}</p><p><strong>{{detail?.auto_blank_pages||0}} automatically marked blank · {{detail?.suspect_text_pages||0}} text pages need a look · {{detail?.unclassified_pages||0}} other scans need a decision · {{detail?.missing_text_pages||0}} need text repair</strong></p></div>
     <div class="actions"><button :class="{selected:needsAttention}" @click="needsAttention=true;selectedPage=null;loadPages()">Needs your decision</button><button :class="{selected:!needsAttention}" @click="needsAttention=false;selectedPage=null;loadPages()">Browse scans</button><template v-if="!needsAttention"><button :disabled="pageOffset===0" @click="movePages(-30)">Previous</button><span>Scans {{pageOffset+1}}–{{Math.min(pageOffset+30,active.pages_total)}}</span><button :disabled="pageOffset+30>=active.pages_total" @click="movePages(30)">Next</button></template><label>Go to scan <input v-model.number="scanNumber" type="number" min="1" :max="active.pages_total" /></label><button @click="jumpToScan">Go</button><label v-if="!needsAttention" class="check"><input v-model="showOmitted" type="checkbox" @change="loadPages" /> Show omitted scans</label></div>
     <p v-if="needsAttention&&!pages.length" class="good">No uncertain scans are waiting for a decision.</p><div class="page-list"><button v-for="p in pages" :key="p.id" :class="{selected:selectedPage?.id===p.id}" @click="selectPage(p)">Scan {{p.scan_page_index+1}} <small>{{p.printed_label?'page '+p.printed_label:'page unlabeled'}} · {{pageState(p)}}</small></button></div>
     <div v-if="selectedPage" :key="selectedPage.id" class="page-review"><h3>Scan {{selectedPage.scan_page_index+1}} <small>{{selectedPage.printed_label?'· printed page '+selectedPage.printed_label:''}}</small></h3><p v-if="selectedPage.triage_reason">{{selectedPage.triage_reason}}</p><p v-if="selectedPage.text_qa_status==='suspect'">{{selectedPage.text_qa_reason}}</p><p v-if="selectedPage.page_kind==='unclassified'&&selectedPage.review_note">Earlier note: {{selectedPage.review_note}}. Check this scan again before saving a decision.</p><div class="compare"><div><a :href="selectedPage.image_url" target="_blank" rel="noreferrer">Open full scan in a new tab</a><img :src="selectedPage.image_url" :alt="`Original scan ${selectedPage.scan_page_index+1}`" /></div><div><b>Stored text</b><pre>{{selectedPage.text||'No text found. Check whether this scan is blank, illustrated, or missing readable text.'}}</pre></div></div><label class="note-label">Printed page label <input v-model="printedLabel" placeholder="e.g. xiv or 12" /></label><button :disabled="busy" @click="savePrintedLabel">Save page label</button><label class="note-label">What does this scan contain? <select v-model="pageOutcome"><option value="" disabled>Choose an outcome</option><option value="text">Text page — keep searchable</option><option value="blank">Blank page — omit from answers</option><option value="illustration">Cover, diagram or illustration — keep visible</option><option value="book_info">Library or book information — omit from answers</option><option value="missing_text">Printed text is missing or incomplete — needs repair</option></select></label><label v-if="pageNoteRequired" class="note-label">What did you check? <textarea v-model="pageNote" rows="2" placeholder="Describe what you see and any missing lines or page-number differences" /></label><button :disabled="busy||!pageOutcome||(pageNoteRequired&&!pageNote.trim())" @click="reviewPage">Save this page review</button></div>

     <div class="step"><h3>2. Check permission to use the scan</h3><p v-if="detail?.rights_mark">Repository mark: “{{detail.rights_mark}}”. <a v-if="detail.rights_evidence_url" :href="detail.rights_evidence_url" target="_blank" rel="noreferrer">Read the rights statement</a>.</p><p v-if="detail?.rights_statement">Recorded rights statement: {{detail.rights_statement}}</p><p v-if="detail?.source_url"><a :href="detail.source_url" target="_blank" rel="noreferrer">Open catalogue record</a></p><p v-if="detail?.pdf_origin_url"><a :href="detail.pdf_origin_url" target="_blank" rel="noreferrer">Open original PDF link</a></p><p>Check the original source and record whether this local research use is permitted.</p><label>Your decision <select v-model="rightsDecision"><option value="" disabled>Choose after checking</option><option value="allowed">Allowed for this use</option><option value="denied">Not allowed</option></select></label><label class="note-label">Reason for the decision <textarea v-model="rightsNote" rows="2" placeholder="Record the evidence and any limits" /></label><button :disabled="busy||!rightsDecision||!rightsNote.trim()" @click="saveRights">Save decision</button><p v-if="detail?.rights_status==='allowed'" class="good">Permission decision saved as allowed.</p></div>
     <div class="step"><h3>3. Make the book available</h3><template v-if="!canPublish"><p>Before this button becomes available:</p><ul><li v-for="need in publicationNeeds" :key="need">{{need}}</li></ul></template><p v-else>Required checks are recorded. This book can now be searched for answers.</p><button v-if="session.role==='admin'" :disabled="busy||!canPublish" @click="publish">Make available for questions</button></div>
    </template>
   </template>
  </section>

  <section v-else-if="tab==='activity'">
   <h1>Activity</h1><p>Saved questions continue while the services are running. Open a finished answer or retry a failed one.</p>
   <p v-if="!answerJobs.length&&!sourceJobs.length" class="empty">No activity yet. Add a source or ask a question to start.</p>
   <h2 v-if="sourceJobs.length">Source preparation</h2>
   <article v-for="group in sourceActivityGroups" :key="group.source_id" class="step"><h3>{{group.title}}</h3><p>{{group.jobs[0].stage}} · {{group.jobs[0].completed}} of {{group.jobs[0].total}} · {{sources.find(source=>source.id===group.source_id)?.status==='review'?'Needs your review':sources.find(source=>source.id===group.source_id)?.status==='published'&&indexBySource[group.source_id]?.status==='ready'?'Ready to ask':group.jobs[0].status==='done'?'Finished':group.jobs[0].status==='running'?'Working':group.jobs[0].status==='queued'?'Waiting':'Needs your attention'}}</p><p v-if="group.jobs[0].error" class="error">{{group.jobs[0].error}}</p><p>Updated {{new Date(group.jobs[0].updated_at).toLocaleString()}}</p><button @click="openSourceJob(group.jobs[0])">View source</button><details v-if="group.jobs.length>1"><summary>Completed and earlier steps</summary><p v-for="job in group.jobs.slice(1)" :key="job.id">{{job.stage}} · {{job.completed}} of {{job.total}} · {{job.status}}</p></details></article>
   <h2 v-if="answerJobs.length">Questions</h2>
   <article v-for="job in answerJobs" :key="job.id" class="step"><h2>{{job.question}}</h2><p>{{job.stage}} · Updated {{new Date(job.updated_at).toLocaleString()}}</p><p v-if="job.status==='retrying'">This answer was interrupted. Your question is saved and will retry automatically.</p><p v-if="job.status==='failed'" class="error">{{job.error_message||'The cause is not yet known.'}} Your question is saved. {{job.error_code==='local_model_unavailable'?'Check LM Studio, then try again.':'Review the selected sources or model provider, then try again.'}}</p><button v-if="job.answer_id" @click="openAnswerJob(job)">Open answer</button><button v-else-if="job.status==='failed'&&job.error_code==='source_unavailable'" @click="reviseAnswerJob(job)">Change sources</button><button v-else-if="job.status==='failed'" :disabled="busy" @click="retryAnswerJob(job)">Try again</button><button v-if="session.role==='admin'" :disabled="busy" @click="openEvaluation(job)">Evaluation report</button></article>
   <section v-if="session.role==='admin'&&evaluationReport" class="step evaluation-report"><div class="sectionhead"><h2>Evaluation report</h2><div class="actions"><button @click="downloadEvaluation">Download JSON</button><button @click="evaluationReport=null">Close</button></div></div><p><strong>Job {{evaluationReport.job_id}}</strong> · {{evaluationReport.owner}} · {{evaluationReport.job_status}} · {{evaluationReport.research_mode}} · {{evaluationReport.attempts}} attempt(s)</p><p>Submitted {{new Date(evaluationReport.created_at).toLocaleString()}}<span v-if="evaluationReport.finished_at"> · Finished {{new Date(evaluationReport.finished_at).toLocaleString()}}</span></p><h3>User input</h3><pre>{{evaluationReport.question_raw}}</pre><h3>System answer</h3><p>{{evaluationReport.answer||'No answer saved yet.'}}</p><p>Status: {{evaluationReport.answer_status||evaluationReport.stage}}</p><h3>RAG metrics</h3><p v-if="evaluationReport.answer_status">{{evaluationReport.metrics.search_count}} searches · {{evaluationReport.metrics.retrieved_passages_across_searches}} retrieved across searches · {{evaluationReport.metrics.selected_candidate_count}} selected candidates · {{evaluationReport.metrics.cited_passage_count}} cited · {{Math.round(evaluationReport.metrics.citation_coverage*100)}}% candidate citation coverage · {{evaluationReport.metrics.supported_claim_count}} supported claims · {{evaluationReport.metrics.rejected_claim_count}} rejected claims</p><p v-else>Metrics will appear when an answer is saved.</p><p v-if="evaluationReport.answer_status">Top retrieval score {{evaluationReport.metrics.top_retrieval_score.toFixed(3)}} · Mean {{evaluationReport.metrics.mean_retrieval_score.toFixed(3)}}</p><p>Answer model: {{evaluationReport.answer_model||'not recorded'}} {{evaluationReport.answer_revision}} · Embedding model: {{evaluationReport.embedding_model||'not recorded'}} {{evaluationReport.embedding_revision}} · Prompt: {{evaluationReport.prompt_revision||'not recorded'}}</p><details><summary>Searches and retrieved passages</summary><p v-for="search in evaluationReport.searches" :key="search.ordinal">{{search.ordinal}}. {{search.query}} · {{search.scope}} · {{search.candidate_count}} found</p><article v-for="candidate in evaluationReport.candidates" :key="candidate.chunk_id"><p><strong>{{candidate.rank}}. {{candidate.title}}</strong> by {{candidate.author}} · scan {{candidate.scan}} · score {{candidate.score.toFixed(3)}} · {{candidate.cited?'cited':'not cited'}}</p><blockquote>{{candidate.preview}}</blockquote></article></details><details><summary>Claim checks ({{evaluationReport.claims.length}})</summary><p v-for="claim in evaluationReport.claims" :key="claim.ordinal">{{claim.ordinal}}. {{claim.decision}} · {{claim.claim}}</p></details><details open><summary>Technical log ({{evaluationReport.logs.length}})</summary><p v-for="(entry,index) in evaluationReport.logs" :key="index">{{new Date(entry.created_at).toLocaleString()}} · attempt {{entry.attempt}} · {{entry.stage}}<span v-if="entry.duration_ms!==null"> · {{entry.duration_ms}} ms</span><br><code>{{JSON.stringify(entry.detail)}}</code></p></details></section>
   <div class="actions"><button :disabled="activityPage===1" @click="changeActivityPage(-1)">Previous</button><span>Page {{activityPage}}</span><button :disabled="answerJobs.length<20&&sourceJobs.length<20" @click="changeActivityPage(1)">Next</button></div>
  </section>
  <section v-else>
   <h1>Ask your sources</h1>
   <p>Searches {{readyCount}} prepared {{readyCount===1?'source':'sources'}}. Ask about a source, what an author reports, or how authors compare. Open each citation to check the passage against its original scan. Historical claims are not modern clinical evidence.</p>
   <div v-if="readyCount===0" class="empty">Prepare a source before asking questions. <button @click="tab='sources'">View sources</button></div>
   <form v-else @submit.prevent="ask">
    <label for="question">Your question</label><textarea id="question" v-model="question" rows="3" maxlength="1000" placeholder="How do Nash and Farrington describe Lachesis?"/>
    <p v-if="question.trim()&&!validQuestion" class="input-hint">Add a topic or remedy name with at least 3 letters, such as “mood” or “Nux”.</p>
    <fieldset class="source-picker"><legend>Sources to search</legend><label><input type="radio" name="source-mode" :checked="sourceSelectionMode==='all'" @change="setSourceSelectionMode('all')" /> All {{readyCount}} prepared sources</label><label><input type="radio" name="source-mode" :checked="sourceSelectionMode==='selected'" @change="setSourceSelectionMode('selected')" /> Choose sources</label><div v-if="sourceSelectionMode==='selected'" class="source-options"><label v-for="source in readySources" :key="source.id"><input v-model="selectedSourceIds" type="checkbox" :value="source.id" /><span><strong>{{source.title}}</strong><br />{{source.author}}</span></label><p>{{selectedSourceIds.length}} selected</p><p v-if="!selectionValid" class="input-hint">Select at least one prepared source. Refresh if a source is no longer ready.</p></div></fieldset>
    <label for="research-mode">Research depth</label><select id="research-mode" v-model="researchMode"><option value="quick">Quick answer · one search per source</option><option value="deep">Detailed research · three focused searches</option></select>
    <button :disabled="busy||!validQuestion||!selectionValid">{{busy?'Saving question…':researchMode==='deep'?'Research in detail':'Ask'}}</button>
   </form>
   <div v-if="!answer&&questionJob" class="step" role="status"><p><strong>Your question:</strong> {{questionJob.stage}}.</p><p v-if="questionJob.status==='working'||questionJob.status==='waiting'||questionJob.status==='retrying'">Your question is saved. The answer will appear here when it is ready.</p><p v-if="questionJob.status==='failed'" class="error">{{questionJob.error_message||'The answer could not be completed.'}}</p><button v-if="questionJob.answer_id" @click="openAnswerJob(questionJob)">Open answer</button><button v-else-if="questionJob.status==='failed'&&questionJob.error_code==='source_unavailable'" @click="reviseAnswerJob(questionJob)">Change sources</button><button v-else-if="questionJob.status==='failed'" :disabled="busy" @click="retryAnswerJob(questionJob)">Try again</button></div>
   <article v-if="answer" class="answer">
    <h2>{{answerStatus==='insufficient_evidence'?'Not enough support yet':answerStatus==='partial'?'Partial answer':sections.length?'Research answer':'Answer from your sources'}}</h2>
    <p v-if="omittedClaims" class="verification-note">{{omittedClaims}} {{omittedClaims===1?'draft sentence was':'draft sentences were'}} left out because the source citation could not be verified or the sentence did not answer your question. Only checked sentences appear below.</p>
    <template v-if="answerStatus!=='insufficient_evidence'&&sections.length">
     <section v-for="(section,index) in sections" :key="index" class="research-section">
      <h3>{{section.title}}</h3>
      <p class="response-text"><template v-for="(part,i) in sectionParts(section.body)" :key="i"><button v-if="citationByLabel[part]" class="inline-citation" @click="showCitation(citationByLabel[part].id)">{{part}}</button><span v-else>{{part}}</span></template></p>
     </section>
    </template>
    <template v-else>
     <div v-for="(parts,paragraphIndex) in answerParagraphs" :key="paragraphIndex"><h3 v-if="answerStatus!=='insufficient_evidence'&&answerParagraphs.length>1">{{paragraphIndex===0?'Research synthesis':'Findings in the sources'}}</h3><p class="response-text"><template v-for="(part,i) in parts" :key="i"><button v-if="citationByLabel[part]" class="inline-citation" @click="showCitation(citationByLabel[part].id)">{{part}}</button><span v-else>{{part}}</span></template></p></div>
    </template>
    <section v-if="searches.length" class="source-refs"><h3>Research trail</h3><p v-for="(search,i) in searches" :key="i">{{search.source_scope==='all'?'All prepared sources':search.source_scope==='selected'?'Selected sources':search.source_scope}} · {{search.candidate_count}} retrieved passages · “{{search.query}}”</p></section>
    <details v-if="claimChecks.length" class="source-refs"><summary>Claim checks and exact source excerpts</summary><article v-for="check in claimChecks" :key="check.ordinal"><p><strong>{{check.decision==='supported'?'Supported':check.decision==='irrelevant'?'Left out: unrelated to question':check.decision==='missing_excerpt'?'Left out: no exact excerpt':'Left out: unsupported'}}</strong> · {{check.claim}}</p><blockquote v-for="item in check.supports" :key="item.label">{{item.label}} · “{{item.excerpt}}”</blockquote></article></details>
    <section v-if="sourceRows.length" class="source-refs"><h3>Sources in this answer</h3><div class="source-table"><div v-for="row in sourceRows" :key="row.author+row.title"><strong>{{row.author}}</strong><span>{{row.title}}</span><span>{{row.count}} {{row.count===1?'cited passage':'cited passages'}} · {{row.pages.join(', ')}}</span></div></div></section>
    <section v-if="citations.length" class="source-refs"><h3>Open cited passages</h3><button v-for="c in citations" :key="c.id" @click="showCitation(c.id)">{{c.label}} · {{c.author}} · {{c.printed_page?'page '+c.printed_page:'scan '+c.scan_position}}</button></section>
    <section v-if="answerStatus!=='insufficient_evidence'" class="source-refs"><h3>Scope and next checks</h3><p>This answer uses prepared sources. Open the cited scans to check context. Historical descriptions alone do not establish present-day clinical evidence.</p></section>
   </article>
  </section>
  <section v-if="tab==='ask'&&evidence.length" class="evidence-section"><h2>Evidence passages</h2><p v-if="citedEvidence.length">These passages support the displayed answer. Open a scan to check its context.</p><div v-if="citedEvidence.length" class="evidence-grid"><article v-for="item in citedEvidence" :key="item.rank" class="evidence-card"><div class="evidence-head"><b>{{item.author}}</b><span>Cited</span></div><p>{{item.title}} · {{item.printed_page?'page '+item.printed_page:'scan '+item.scan_position}}</p><blockquote>{{item.preview}}</blockquote><a :href="item.image_url" target="_blank" rel="noreferrer">Open original scan {{item.scan_position}}</a></article></div><details v-if="uncitedEvidence.length"><summary>Show {{uncitedEvidence.length}} retrieved {{uncitedEvidence.length===1?'passage':'passages'}} not used in the answer</summary><p>Search retrieved these candidates, but they did not support the displayed answer. Some may be unrelated to your question.</p><div class="evidence-grid"><article v-for="item in uncitedEvidence" :key="item.rank" class="evidence-card"><div class="evidence-head"><b>{{item.author}}</b><span>Not cited</span></div><p>{{item.title}} · {{item.printed_page?'page '+item.printed_page:'scan '+item.scan_position}}</p><blockquote>{{item.preview}}</blockquote><a :href="item.image_url" target="_blank" rel="noreferrer">Open original scan {{item.scan_position}}</a></article></div></details></section>
  <aside v-if="citation" class="drawer" aria-label="Source reference"><button class="close" @click="citation=null">Close</button><h2>{{citation.title}}</h2><p>{{citation.author}} · {{citation.printed_page?'printed page '+citation.printed_page:'scan '+citation.scan_position}}</p><blockquote>{{citation.passage}}</blockquote><img class="citation-scan" :src="citation.image_url" :alt="`Original scan ${citation.scan_position}`" /><p><a :href="citation.pdf_url" target="_blank" rel="noreferrer">Open saved PDF at this page</a></p><p v-if="citation.source_url"><a :href="citation.source_url" target="_blank" rel="noreferrer">Open source record or DOI</a></p><a :href="citation.image_url" target="_blank" rel="noreferrer">Open full scan</a></aside>
  </template>
 </main>
</template>
