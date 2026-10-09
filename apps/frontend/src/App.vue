<script setup lang="ts">
import {computed,nextTick,onMounted,onUnmounted,ref} from 'vue'
import {api,ApiError,type Citation,type Page,type Source} from './api'
import SourceLibrary, {type LibraryGroup} from './components/SourceLibrary.vue'

type SourceDetail=Source&{document_blocks:number,document_blocks_reviewed:number,requested_url:string,final_url:string,transport:string,acquired_at:string,pdf_sha256:string,pages_reviewed:number,unclassified_pages:number,missing_text_pages:number,suspect_text_pages:number,text_pages_checked:number,text_pages_total:number,text_qa_status:string,text_qa_error:string,auto_blank_pages:number,triage_status:string,triage_error:string,triage_completed:number,triage_total:number,review_coverage:{front:boolean,beginning:boolean,middle:boolean,end:boolean},rights_mark:string,rights_evidence_url:string,source_url:string,pdf_origin_url:string,edition:string,publication_info:string,repository:string,rights_statement:string,retraction_notice_url:string,edition_id:string,source_record_id:string,source_asset_id:string,processing_revision_id:string,published_revision_id:string|null,rights_decision_id:string|null,suggested_categories:string[],classification_state:string,classification_reason:string,classifier_version:string,literature_category_origin:string}
type DocumentBlock={id:string,block_index:number,section_key:string,kind:string,heading:string,original_text:string,reviewed_text:string,start_byte:number,end_byte:number,review_status:string,review_note:string,warnings:string[],literature_categories:string[],category_override:boolean}
type DOIReference={id:string,doi:string,title:string,authors:string,publication_year:number,publisher:string,work_type:string,doi_url:string,pdf_available:boolean,license_url:string,metadata_provider:string,source_id:string,retraction_notice_url:string}
type ArchiveWork={identifier:string,title:string,creator:string,year:string,record_url:string}
type ArchivePDF={name:string,url:string,bytes:number,source:string}
type ArchiveItem=ArchiveWork&{publication_info:string,rights:string,license_url:string,pdfs:ArchivePDF[]}
const tab=ref<'ask'|'sources'|'review'|'activity'>('sources')
const intakeOpen=ref(false),intakeMethod=ref<'upload'|'document'|'link'|'archive'|'doi'>('upload')
const sourcesLoading=ref(true),uploadDragging=ref(false),uploadInput=ref<HTMLInputElement|null>(null)
const intakeSuccess=ref(''),lastImportedSource=ref('')
const intakeMethods=[{id:'upload',name:'Upload PDF',hint:'From your computer'},{id:'document',name:'Upload HTML/TXT',hint:'One static document'},{id:'link',name:'Source link',hint:'Preview a public URL'},{id:'archive',name:'Find a book',hint:'Search Internet Archive'},{id:'doi',name:'Add a paper',hint:'Look up a DOI'}] as const
const sources=ref<Source[]>([]),active=ref<Source|null>(null),detail=ref<SourceDetail|null>(null)
const doiReferences=ref<DOIReference[]>([]),doiInput=ref('')
const doiNotice=ref(''),lastDOIID=ref('')
const archiveQuery=ref(''),archiveResults=ref<ArchiveWork[]>([]),archiveItem=ref<ArchiveItem|null>(null),archivePage=ref(1),archiveTotal=ref(0),archiveNotice=ref('')
const pages=ref<Page[]>([]),pageOffset=ref(0),scanNumber=ref(1),selectedPage=ref<Page|null>(null)
const documentFile=ref<File|null>(null),documentCharset=ref(''),intakeLiteratureCategory=ref(''),documentBlocks=ref<DocumentBlock[]>([]),blockTexts=ref<Record<string,string>>({}),blockNotes=ref<Record<string,string>>({})
const literatureOptions=['materia_medica','repertory','organon_philosophy','therapeutics','provings','clinical_cases','research','other','unclassified']
const evidenceOptions=['unknown','classical_reference','published_case','trial_study','systematic_review','trial_registration','guideline_safety','other']
const selectedLiteratureCategories=ref<string[]>([]),reviewLiteratureCategories=ref<string[]>(['unclassified']),reviewEvidenceCategory=ref('unknown'),categoryReason=ref(''),sectionCategories=ref<Record<string,string[]>>({}),sectionCategoryReasons=ref<Record<string,string>>({})
const pageNote=ref(''),pageOutcome=ref(''),showOmitted=ref(false),needsAttention=ref(false),rightsNote=ref(''),rightsDecision=ref('')
const pageNoteRequired=computed(()=>pageOutcome.value!=='blank'&&pageOutcome.value!=='book_info')
const uploadFile=ref<File|null>(null),uploadTitle=ref(''),uploadAuthor=ref(''),uploadEdition=ref(''),uploadPublication=ref(''),uploadRepository=ref(''),uploadURL=ref(''),uploadRights=ref('')
const uploadNotice=ref('')
type URLPreview={requested_url:string,final_url:string,redirected:boolean,transport_changed:boolean,unencrypted:boolean,detected_type:string,content_type:string,byte_size:number,sample:string,readiness:string,can_import_pdf:boolean,block_count:number,charset:string,extraction_warnings:string[]}
const linkPreview=ref<URLPreview|null>(null),previewError=ref(''),previewBusy=ref(false),allowHTTPRedirect=ref(false)
const previewCharset=ref('')
const linkPDF=ref(''),linkTitle=ref(''),linkAuthor=ref(''),linkEdition=ref(''),linkPublication=ref(''),linkRepository=ref(''),linkSourceURL=ref(''),linkRights=ref(''),linkNotice=ref('')
const metadata=ref({title:'',author:'',edition:'',publication_info:'',repository:'',source_url:'',rights_statement:''}),printedLabel=ref('')
const sourceAccessReason=ref('')
type AnswerCitation={id:string,label:string,title:string,author:string,printed_page:string,scan_position:number,literature_categories?:string[],evidence_category?:string,format?:string}
type SearchRecord={query:string,source_scope:string,candidate_count:number}
type EvidenceRecord={rank:number,title:string,author:string,printed_page:string,scan_position:number,image_url:string,preview:string,cited:boolean,literature_categories?:string[],evidence_category?:string,format?:string}
type ResearchSection={title:string,body:string}
const question=ref(''),answer=ref(''),answerStatus=ref(''),citations=ref<AnswerCitation[]>([]),citation=ref<Citation|null>(null)
const answerCategoryScope=ref<string[]>([])
const validQuestion=computed(()=>/[\p{L}\p{N}]{3,}/u.test(question.value))
const researchMode=ref<'quick'|'deep'>('quick')
const sourceSelectionMode=ref<'all'|'selected'>('all'),selectedSourceIds=ref<string[]>([])
const asksAboutSelectedSource=computed(()=>/\bselected (study|paper|source)\b/i.test(question.value))
const searches=ref<SearchRecord[]>([])
const evidence=ref<EvidenceRecord[]>([])
const citedEvidence=computed(()=>evidence.value.filter(item=>item.cited))
const uncitedEvidence=computed(()=>evidence.value.filter(item=>!item.cited))
const sections=ref<ResearchSection[]>([])
const omittedClaims=ref(0)
type AnswerJob={id:string,question:string,research_mode:string,status:string,stage:string,attempts:number,answer_id:string|null,error_code:string|null,error_message:string|null,created_at:string,updated_at:string}
type EvaluationReport={job_id:string,question_raw:string,answer:string,answer_status:string,job_status:string,stage:string,attempts:number,owner:string,research_mode:string,selected_source_ids:string[],created_at:string,finished_at:string|null,answer_model:string,answer_revision:string,prompt_revision:string,embedding_model:string,embedding_revision:string,metrics:{search_count:number,retrieved_passages_across_searches:number,selected_candidate_count:number,cited_passage_count:number,citation_coverage:number,supported_claim_count:number,rejected_claim_count:number,top_retrieval_score:number,mean_retrieval_score:number},searches:{ordinal:number,query:string,scope:string,candidate_count:number}[],candidates:{rank:number,score:number,chunk_id:string,title:string,author:string,scan:number,printed_page:string,preview:string,cited:boolean}[],claims:{ordinal:number,claim:string,decision:string,check_method:string}[],logs:{attempt:number,stage:string,detail:Record<string,unknown>,duration_ms:number|null,created_at:string}[],model_calls:{kind:string,provider:string,requested_model:string,returned_model:string,request_id:string,outcome:string,prompt_tokens:number|null,completion_tokens:number|null,total_tokens:number|null,reasoning_tokens:number|null,estimated_cost_usd:number|null,cost_origin:string,duration_ms:number,created_at:string}[],model_usage_summary:{call_count:number,known_estimated_cost_usd:number,unknown_cost_call_count:number}}
type SourceJob={id:string,source_id:string,title:string,kind:string,status:string,stage:string,completed:number,total:number,attempts:number,error:string|null,updated_at:string}
type SavedAnswer={answer_id:string,question?:string,answer:string,status:string,citations:AnswerCitation[],searches?:SearchRecord[],evidence?:EvidenceRecord[],sections?:ResearchSection[],omitted_claim_count?:number,literature_categories?:string[]}
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
const passageLocation=(item:{printed_page?:string,scan_position?:number,format?:string})=>item.format&&item.format!=='pdf'||!item.scan_position?`section ${item.printed_page||'in document'}`:item.printed_page?`page ${item.printed_page}`:`scan ${item.scan_position}`
const citationPart=(item:Citation,start:number,end?:number)=>Array.from(item.reader_text||'').slice(start,end).join('')
const citationByLabel=computed(()=>Object.fromEntries(citations.value.map(c=>[`[${c.label}]`,c])))
const sourceRows=computed(()=>{
 const grouped=new Map<string,{author:string,title:string,pages:string[],count:number}>()
 for(const c of citations.value){
  const key=`${c.author}|${c.title}`
  const row=grouped.get(key)||{author:c.author,title:c.title,pages:[],count:0}
  const page=passageLocation(c)
  if(!row.pages.includes(page))row.pages.push(page)
  row.count++
  grouped.set(key,row)
 }
 return [...grouped.values()]
})
type IndexStatus={status:string,model:string,completed:number,total:number,error:string,can_retry:boolean,matches_config:boolean}
const index=ref<IndexStatus|null>(null),indexBySource=ref<Record<string,IndexStatus>>({})
const connectionError=ref(''),actionError=ref(''),busy=ref(false)
const session=ref<{name:string,role:string}|null>(null),sessionLoading=ref(true),accessToken=ref(''),signInError=ref(''),signingIn=ref(false),requiredRole=ref<''|'admin'>('')
const username=ref(''),password=ref(''),loginMode=ref<'admin'|'reviewer'>('admin')
let authVersion=0
async function signIn(){
 signingIn.value=true;signInError.value=''
 try{session.value=await api<{name:string,role:string}>('/session',{method:'POST',body:JSON.stringify(loginMode.value==='admin'?{username:username.value,password:password.value,required_role:'admin'}:{token:accessToken.value,required_role:requiredRole.value})});authVersion++;password.value='';accessToken.value='';requiredRole.value='';reconcileNow()}
 catch(e){password.value='';accessToken.value='';signInError.value=e instanceof Error?e.message:'Could not sign in.'}
 finally{signingIn.value=false}
}
async function signOut(){
 try{await api('/session',{method:'DELETE'})}catch{actionError.value='Could not sign out. Check the service and try again.';return}
 authVersion++
 session.value=null;requiredRole.value='';loginMode.value='admin';password.value='';accessToken.value='';sources.value=[];answerJobs.value=[];sourceJobs.value=[];answer.value='';citation.value=null;actionError.value='';connectionError.value='';currentJobID.value=''
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
const readySources=computed(()=>sources.value.filter(s=>sourceGroup(s)==='ready'))
const librarySources=computed(()=>sources.value.map(s=>({...s,group:sourceGroup(s),state:stateText(s)})))
const activeCandidate=computed(()=>active.value?sources.value.find(s=>s.supersedes_source_id===active.value?.id&&s.status!=='failed'&&s.status!=='disabled'):null)
const readyCount=computed(()=>readySources.value.length)
const selectionValid=computed(()=>sourceSelectionMode.value==='all'?!asksAboutSelectedSource.value:selectedSourceIds.value.length>0&&(!asksAboutSelectedSource.value||selectedSourceIds.value.length===1)&&selectedSourceIds.value.every(id=>readySources.value.some(s=>s.id===id)))
const reviewReady=computed(()=>detail.value?.status==='review'&&!detail.value?.retraction_notice_url)
const publicationNeeds=computed(()=>{
 const d=detail.value
 if(!d)return []
 const needs:string[]=[]
	if(d.document_format!=='pdf'){
	 if(d.title.startsWith('Untitled document (verify details)')||d.author.startsWith('Unknown author (verify details)'))needs.push('Confirm title and author')
	 if(d.document_blocks===0)needs.push('Wait for text extraction')
	 if(d.document_blocks_reviewed<d.document_blocks)needs.push(`Review ${d.document_blocks-d.document_blocks_reviewed} blocks`)
	 if(!documentBlocks.value.some(b=>b.review_status==='accepted'||b.review_status==='corrected'))needs.push('Accept at least one text block')
	 if(d.rights_status!=='allowed')needs.push('Save an allowed rights decision')
	 return needs
	}
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
 if(s.rights_status==='denied')return 'Permission denied'
 if(s.supersedes_source_id&&s.status!=='published')return `Candidate revision · ${s.status==='review'?'needs review':s.status==='processing'?`${s.pages_read} of ${s.pages_total} pages`:s.status}`
 if(s.status==='queued'||s.status==='processing')return `Reading scanned pages · ${s.pages_read} of ${s.pages_total}`
 if(s.status==='review')return 'Needs your review'
 if(s.status==='published'&&indexBySource.value[s.id]?.status==='unpublished')return 'Needs passage preparation'
 if(s.status==='published')return sourceGroup(s)==='ready'?'Ready to ask':indexBySource.value[s.id]?.status==='ready'&&!indexBySource.value[s.id]?.matches_config?'Needs reindexing':indexBySource.value[s.id]?.status==='failed'?'Preparation stopped':s.rights_status!=='allowed'?'Check permission':'Preparing for questions'
 if(s.status==='failed')return 'Reading stopped'
 return s.status
}
function sourceGroup(s:Source):LibraryGroup{
 if(s.superseded||s.status==='disabled'||s.rights_status==='denied'||doiReferences.value.some(d=>d.source_id===s.id&&d.retraction_notice_url))return 'unavailable'
 const idx=indexBySource.value[s.id]
 if(s.status==='published'&&s.rights_status==='allowed'&&idx?.status==='ready'&&idx.matches_config)return 'ready'
 if(s.status==='review'||s.status==='failed'||(s.status==='published'&&(s.rights_status!=='allowed'||idx?.status==='failed'||idx?.status==='unpublished'||(idx?.status==='ready'&&!idx.matches_config))))return 'attention'
 return 'processing'
}
async function openIntake(){intakeOpen.value=true;await nextTick();document.getElementById('source-intake')?.scrollIntoView({behavior:'smooth',block:'start'});document.getElementById('intake-heading')?.focus({preventScroll:true})}
async function importedSource(result:{source_id:string,title:string}){
	intakeSuccess.value=`“${result.title}” was added. Processing continues in the background; open the source to follow its progress.`
 lastImportedSource.value=result.source_id;intakeOpen.value=false
 await refresh();await nextTick();document.getElementById('intake-success')?.focus()
}
function openImportedSource(){const source=sources.value.find(s=>s.id===lastImportedSource.value);if(source)void chooseSource(source)}
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
    if(detail.value.document_format!=='pdf'&&detail.value.document_blocks!==documentBlocks.value.length)await loadDocumentBlocks()
    if(needsAttention.value&&previous!==undefined&&(previous!==detail.value.triage_completed||detail.value.suspect_text_pages!==pages.value.filter(p=>p.text_qa_status==='suspect').length))await loadPages()
   }
  }
  connectionError.value=''
 }catch(e){connectionError.value=e instanceof Error?e.message:'Could not connect to the service. Try again when it is running.'}
 finally{sourcesLoading.value=false}
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
async function previewSourceURL(){
 if(!linkPDF.value.trim())return
 previewBusy.value=true;previewError.value='';linkPreview.value=null
 try{linkPreview.value=await api<URLPreview>('/sources/preview-url',{method:'POST',body:JSON.stringify({url:linkPDF.value.trim(),allow_https_to_http_redirect:allowHTTPRedirect.value,charset_override:previewCharset.value})})}
 catch(e){previewError.value=e instanceof Error?e.message:'Could not preview this URL.'}
 finally{previewBusy.value=false}
}
function importPDFLink(){if(!linkPDF.value.trim())return;linkNotice.value='';run(async()=>{
	const isDocument=linkPreview.value?.detected_type==='html'||linkPreview.value?.detected_type==='txt'
	const result=await api<{source_id:string,title:string,author:string}>(isDocument?'/sources/import-document-url':'/sources/import-url',{method:'POST',body:JSON.stringify({[isDocument?'url':'pdf_url']:linkPDF.value.trim(),title:linkTitle.value.trim(),author:linkAuthor.value.trim(),edition:linkEdition.value.trim(),publication_info:linkPublication.value.trim(),repository:linkRepository.value.trim(),source_url:linkSourceURL.value.trim(),rights_statement:linkRights.value.trim(),charset_override:previewCharset.value,allow_https_to_http_redirect:allowHTTPRedirect.value,...(isDocument&&intakeLiteratureCategory.value?{literature_categories:[intakeLiteratureCategory.value]}:{})})})
 linkPDF.value='';linkTitle.value='';linkAuthor.value='';linkEdition.value='';linkPublication.value='';linkRepository.value='';linkSourceURL.value='';linkRights.value='';intakeLiteratureCategory.value=''
 await importedSource(result)
})}
function setPDF(file:File|null){
 uploadNotice.value='';uploadFile.value=null
 if(!file)return
 if(!/\.pdf$/i.test(file.name)&&file.type!=='application/pdf'){uploadNotice.value='Choose a PDF file.';return}
 if(file.size===0||file.size>250*1024*1024){uploadNotice.value='Choose a non-empty PDF up to 250 MB.';return}
 uploadFile.value=file
}
function pickPDF(event:Event){setPDF((event.target as HTMLInputElement).files?.[0]||null)}
function dropPDF(event:DragEvent){uploadDragging.value=false;if(busy.value)return;if(event.dataTransfer?.files.length!==1){uploadNotice.value='Add one PDF at a time.';return}setPDF(event.dataTransfer.files[0])}
function uploadPDF(){if(!uploadFile.value)return;uploadNotice.value='';run(async()=>{
 const form=new FormData();form.set('file',uploadFile.value!);form.set('title',uploadTitle.value.trim());form.set('author',uploadAuthor.value.trim());form.set('edition',uploadEdition.value.trim());form.set('publication_info',uploadPublication.value.trim());form.set('repository',uploadRepository.value.trim());form.set('source_url',uploadURL.value.trim());form.set('rights_statement',uploadRights.value.trim())
 const result=await api<{source_id:string,title:string,author:string}>('/sources/upload',{method:'POST',body:form});uploadFile.value=null;uploadTitle.value='';uploadAuthor.value='';uploadEdition.value='';uploadPublication.value='';uploadRepository.value='';uploadURL.value='';uploadRights.value='';if(uploadInput.value)uploadInput.value.value=''
 await importedSource(result)
})}
function uploadDocument(){if(!documentFile.value)return;run(async()=>{
 const form=new FormData();form.set('file',documentFile.value!);form.set('title',uploadTitle.value.trim());form.set('author',uploadAuthor.value.trim());form.set('edition',uploadEdition.value.trim());form.set('publication_info',uploadPublication.value.trim());form.set('repository',uploadRepository.value.trim());form.set('source_url',uploadURL.value.trim());form.set('rights_statement',uploadRights.value.trim());form.set('charset_override',documentCharset.value);if(intakeLiteratureCategory.value)form.set('literature_categories',intakeLiteratureCategory.value)
 const result=await api<{source_id:string,title:string}>('/sources/upload-document',{method:'POST',body:form})
 documentFile.value=null;uploadTitle.value='';uploadAuthor.value='';uploadEdition.value='';uploadPublication.value='';uploadRepository.value='';uploadURL.value='';uploadRights.value='';intakeLiteratureCategory.value='';await importedSource(result)
})}
async function loadDocumentBlocks(){if(!active.value)return;try{documentBlocks.value=await api<DocumentBlock[]>(`/sources/${active.value.id}/blocks`);blockTexts.value=Object.fromEntries(documentBlocks.value.map(b=>[b.id,b.reviewed_text]));sectionCategories.value=Object.fromEntries(documentBlocks.value.map(b=>[b.id,[...b.literature_categories]]));connectionError.value=''}catch(e){connectionError.value=e instanceof Error?e.message:'Could not load document blocks.'}}
function reviewDocumentBlock(block:DocumentBlock,decision:'accepted'|'corrected'|'excluded'){run(async()=>{await api(`/document-blocks/${block.id}/review`,{method:'POST',body:JSON.stringify({decision,text:blockTexts.value[block.id],note:blockNotes.value[block.id]||''})});await loadDocumentBlocks()})}
function toggleReviewCategory(value:string){if(value==='unclassified'){reviewLiteratureCategories.value=['unclassified'];return}const current=new Set(reviewLiteratureCategories.value.filter(v=>v!=='unclassified'));if(current.has(value))current.delete(value);else current.add(value);reviewLiteratureCategories.value=current.size?[...current]:['unclassified']}
function toggleAskCategory(value:string){if(value==='unclassified'){selectedLiteratureCategories.value=selectedLiteratureCategories.value.includes('unclassified')?[]:['unclassified'];return}const current=new Set(selectedLiteratureCategories.value.filter(v=>v!=='unclassified'));if(current.has(value))current.delete(value);else current.add(value);selectedLiteratureCategories.value=[...current]}
function toggleSectionCategory(id:string,value:string){if(value==='unclassified'){sectionCategories.value[id]=['unclassified'];return}const current=new Set((sectionCategories.value[id]||['unclassified']).filter(v=>v!=='unclassified'));if(current.has(value))current.delete(value);else current.add(value);sectionCategories.value[id]=current.size?[...current]:['unclassified']}
function saveLiteratureCategories(origin:'accepted_suggestion'|'manual'){if(!active.value)return;run(async()=>{const categories=origin==='accepted_suggestion'?detail.value?.suggested_categories||['unclassified']:reviewLiteratureCategories.value;await api(`/sources/${active.value!.id}/categories`,{method:'PUT',body:JSON.stringify({categories,origin,rationale:categoryReason.value.trim()||'Reviewed extracted content and source identity',evidence_category:reviewEvidenceCategory.value})});reviewLiteratureCategories.value=[...categories];categoryReason.value=''})}
function retryClassification(){if(!active.value)return;run(async()=>{await api(`/sources/${active.value!.id}/categories/retry`,{method:'POST'})})}
function saveSectionCategories(block:DocumentBlock){run(async()=>{await api(`/document-blocks/${block.id}/categories`,{method:'PUT',body:JSON.stringify({categories:sectionCategories.value[block.id],rationale:sectionCategoryReasons.value[block.id]||'Reviewed this section against the saved document'})});await loadDocumentBlocks()})}
async function chooseSource(s:Source){
	active.value=s;detail.value=null;pages.value=[];documentBlocks.value=[];rightsDecision.value='';rightsNote.value='';sourceAccessReason.value='';pageOffset.value=0;selectedPage.value=null;tab.value='review'
 await refresh()
 const current=detail.value as SourceDetail|null
 if(current){metadata.value={title:current.title,author:current.author,edition:current.edition,publication_info:current.publication_info,repository:current.repository,source_url:current.source_url,rights_statement:current.rights_statement};reviewLiteratureCategories.value=[...current.literature_categories];reviewEvidenceCategory.value=current.evidence_category}
	needsAttention.value=true;if(current?.document_format==='pdf')await loadPages();else await loadDocumentBlocks();window.scrollTo({top:0,behavior:'smooth'})
}
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
function reimportDocument(){if(!active.value)return;run(async()=>{const result=await api<{source_id:string}>(`/sources/${active.value!.id}/reimport-document`,{method:'POST',body:JSON.stringify({allow_https_to_http_redirect:false})});await refresh();const candidate=sources.value.find(s=>s.id===result.source_id);if(candidate)await chooseSource(candidate)})}
function setSourceAccess(enable:boolean){if(!active.value||sourceAccessReason.value.trim().length<8)return;run(async()=>{await api(`/sources/${active.value!.id}/${enable?'enable':'disable'}`,{method:'POST',body:JSON.stringify({reason:sourceAccessReason.value.trim()})});sourceAccessReason.value=''})}
function setSourceSelectionMode(mode:'all'|'selected'){sourceSelectionMode.value=mode;if(mode==='selected'&&asksAboutSelectedSource.value&&selectedSourceIds.value.length!==1)selectedSourceIds.value=[]}
function showAnswer(result:SavedAnswer,openAsk=true){
 answer.value=result.answer;answerStatus.value=result.status;citations.value=result.citations;searches.value=result.searches||[];evidence.value=result.evidence||[];sections.value=result.sections||[];omittedClaims.value=result.omitted_claim_count||0
 answerCategoryScope.value=result.literature_categories||[]
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
 const result=await api<{job_id:string}>('/research/answer-jobs',{method:'POST',body:JSON.stringify({question:question.value,mode:researchMode.value,source_ids:sourceSelectionMode.value==='selected'?selectedSourceIds.value:[],literature_categories:selectedLiteratureCategories.value})})
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
function askAboutSource(){if(!active.value||sourceGroup(active.value)!=='ready')return;sourceSelectionMode.value='selected';selectedSourceIds.value=[active.value.id];openAskTab()}
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
  <form v-else-if="!session" class="step" @submit.prevent="signIn">
   <h1>{{loginMode==='admin'?'Sign in as administrator':'Sign in as reviewer'}}</h1>
   <template v-if="loginMode==='admin'">
    <label>Username <input v-model="username" name="username" type="text" autocomplete="username" required /></label>
    <label>Password <input v-model="password" name="password" type="password" autocomplete="current-password" required /></label>
   </template>
   <label v-else>Reviewer access token <input v-model="accessToken" name="token" type="password" autocomplete="current-password" required /></label>
   <button :disabled="signingIn||(loginMode==='admin'?(!username.trim()||!password):!accessToken.trim())">{{signingIn?'Signing in…':'Sign in'}}</button>
   <button v-if="requiredRole!=='admin'" type="button" :disabled="signingIn" @click="loginMode=loginMode==='admin'?'reviewer':'admin';password='';accessToken='';signInError=''">{{loginMode==='admin'?'Sign in as reviewer':'Sign in as administrator'}}</button>
   <p v-if="signInError" class="error" role="alert">{{signInError}}</p>
  </form>
  <template v-else>
  <header><strong>Source research</strong><nav aria-label="Main"><button :class="{selected:tab==='ask'}" @click="openAskTab">Ask</button><button :class="{selected:tab==='sources'}" @click="tab='sources'">Sources</button><button :class="{selected:tab==='review'}" @click="tab='review'">Review</button><button :class="{selected:tab==='activity'}" @click="openActivityTab">Activity <span v-if="runningActivity||attentionActivity||unreadActivity">({{runningActivity}} working · {{attentionActivity}} need attention<span v-if="unreadActivity"> · {{unreadActivity}} new</span>)</span></button><span>{{session.name}} ({{session.role}})</span><button @click="signOut">Sign out</button></nav></header>
  <p v-if="connectionError" class="error" role="alert">{{connectionError}} <button @click="refresh">Try again</button></p>
  <p v-if="actionError&&!(tab==='sources'&&intakeOpen)" class="error" role="alert">{{actionError}} <button @click="actionError=''">Dismiss</button></p>

  <section v-if="tab==='sources'" class="sources-workspace">
   <div class="sources-heading"><div><p class="eyebrow">KNOWLEDGE BASE</p><h1>Sources</h1><p>Build a library you can trace every answer back to.</p></div><button v-if="session.role==='admin'" class="primary-button" :disabled="busy" @click="openIntake"><span aria-hidden="true">＋</span> Add source</button></div>
   <div class="source-flow" aria-label="How sources become searchable"><span><b>1</b> Add a document</span><span aria-hidden="true">→</span><span><b>2</b> Automatic page checks</span><span aria-hidden="true">→</span><span><b>3</b> Review flagged issues & rights</span><span aria-hidden="true">→</span><span><b>4</b> Publish & ask</span></div>
   <div v-if="intakeSuccess" id="intake-success" class="intake-success" role="status" tabindex="-1"><div><strong>Source added</strong><p>{{intakeSuccess}}</p></div><button v-if="sources.some(s=>s.id===lastImportedSource)" @click="openImportedSource">View progress</button><button aria-label="Dismiss source-added message" @click="intakeSuccess=''">×</button></div>
   <section v-if="session.role==='admin'&&intakeOpen" id="source-intake" class="intake-panel" aria-labelledby="intake-heading" :aria-busy="busy">
    <div class="intake-heading"><div><h2 id="intake-heading" tabindex="-1">Add a source</h2><p>Choose how to bring a document into your library.</p></div><button :disabled="busy" aria-label="Close add source" @click="intakeOpen=false">×</button></div>
    <p v-if="actionError" class="intake-error error" role="alert">{{actionError}}</p>
    <div class="intake-methods" aria-label="Source import method"><button v-for="method in intakeMethods" :key="method.id" :disabled="busy" :aria-pressed="intakeMethod===method.id" @click="intakeMethod=method.id"><strong>{{method.name}}</strong><small>{{method.hint}}</small></button></div>
    <div v-show="intakeMethod==='upload'" class="intake-body">
     <form class="intake-form" @submit.prevent="uploadPDF">
      <h3>Upload a PDF</h3><p class="input-hint">We’ll read the document and suggest its title and author. You can correct the details during review.</p>
      <div class="upload-zone" :class="{dragging:uploadDragging}" @dragover.prevent="uploadDragging=true" @dragleave.prevent="uploadDragging=false" @drop.prevent="dropPDF"><span class="upload-symbol" aria-hidden="true">↑</span><strong>{{uploadFile?uploadFile.name:'Drop your PDF here'}}</strong><span v-if="uploadFile">{{(uploadFile.size/1048576).toFixed(1)}} MB · Ready to upload</span><span v-else>or choose a file from your computer</span><label class="file-select">{{uploadFile?'Choose another PDF':'Choose PDF'}}<input ref="uploadInput" type="file" accept="application/pdf,.pdf" :disabled="busy" @change="pickPDF" /></label><small>One PDF at a time · Up to 250 MB</small></div>
      <p v-if="uploadNotice" class="error" role="alert">{{uploadNotice}}</p>
      <details class="optional-details"><summary>Add source details <span>Optional</span></summary><p>Already know the edition or rights statement? Add them now to help the review.</p><div class="metadata-grid"><label>Title<input v-model="uploadTitle" maxlength="300" /></label><label>Author<input v-model="uploadAuthor" maxlength="300" /></label><label>Edition or volume<input v-model="uploadEdition" maxlength="300" /></label><label>Publication details<input v-model="uploadPublication" maxlength="500" /></label><label>Repository or collection<input v-model="uploadRepository" maxlength="300" /></label><label>Source record URL<input v-model="uploadURL" type="url" maxlength="1000" /></label><label class="full-width">Rights statement<textarea v-model="uploadRights" rows="2" maxlength="2000" placeholder="Record the source’s stated permissions" /></label></div></details>
      <div class="intake-footer"><span>After upload, page reading continues in the background.</span><button class="primary-button" :disabled="busy||!uploadFile">{{busy?'Uploading PDF…':'Upload & process'}}</button></div>
     </form>
    </div>
    <div v-show="intakeMethod==='document'" class="intake-body">
     <form class="intake-form" @submit.prevent="uploadDocument"><h3>Upload one HTML or TXT document</h3><p class="input-hint">Static HTML and plain text up to 10 MiB. The saved original stays private; every extracted block needs review.</p><label>Document<input type="file" accept=".html,.htm,.txt,text/html,text/plain" :disabled="busy" @change="documentFile=($event.target as HTMLInputElement).files?.[0]||null" /></label><label>Text encoding override<select v-model="documentCharset"><option value="">Detect automatically</option><option value="utf-8">UTF-8</option><option value="windows-1252">Windows-1252</option><option value="iso-8859-1">ISO-8859-1</option></select></label><label>Literature category at intake<select v-model="intakeLiteratureCategory"><option value="">Let extraction suggest a category</option><option v-for="value in literatureOptions" :key="value" :value="value">{{value.replaceAll('_',' ')}}</option></select></label><div class="metadata-grid"><label>Title<input v-model="uploadTitle" /></label><label>Author<input v-model="uploadAuthor" /></label><label>Edition<input v-model="uploadEdition" /></label><label>Publication details<input v-model="uploadPublication" /></label><label>Repository<input v-model="uploadRepository" /></label><label>Source URL<input v-model="uploadURL" type="url" /></label><label>Rights statement<textarea v-model="uploadRights" rows="2" /></label></div><button class="primary-button" :disabled="busy||!documentFile||documentFile.size>10485760">{{busy?'Uploading…':'Upload for review'}}</button></form>
    </div>
    <div v-show="intakeMethod==='link'" class="intake-body">
     <form class="intake-form" @submit.prevent="importPDFLink"><h3>Preview a source link</h3><p class="input-hint">Preview a public HTTP or HTTPS page, TXT document, or direct PDF. A preview does not add a source.</p><label>Source URL<input v-model="linkPDF" type="url" placeholder="https://example.org/document" required @input="linkPreview=null;previewError=''" /></label><label class="preview-option"><input v-model="allowHTTPRedirect" type="checkbox" /> Allow an HTTPS link to redirect to unencrypted HTTP</label><label>Text encoding override<select v-model="previewCharset" @change="linkPreview=null"><option value="">Detect automatically</option><option value="utf-8">UTF-8</option><option value="windows-1252">Windows-1252</option><option value="iso-8859-1">ISO-8859-1</option></select></label><button type="button" :disabled="previewBusy||!linkPDF.trim()" @click="previewSourceURL">{{previewBusy?'Checking…':'Preview URL'}}</button><p v-if="previewError" class="error" role="alert">{{previewError}}</p><div v-if="linkPreview" class="step" role="status"><p><strong>{{linkPreview.detected_type.toUpperCase()}}</strong> · {{linkPreview.byte_size.toLocaleString()}} bytes · {{linkPreview.readiness}}</p><p>Requested: {{linkPreview.requested_url}}<br />Final: {{linkPreview.final_url}}</p><p v-if="linkPreview.redirected">Redirected{{linkPreview.transport_changed?' with a transport change':''}}.</p><p v-if="linkPreview.unencrypted" class="error">Unencrypted HTTP acquisition.</p><p v-if="linkPreview.charset">Detected encoding: {{linkPreview.charset}} · {{linkPreview.block_count}} extracted blocks</p><p v-for="warning in linkPreview.extraction_warnings" :key="warning" class="error">{{warning}}</p><p v-if="linkPreview.sample">Sample: {{linkPreview.sample}}</p><p v-if="linkPreview.detected_type==='xml'">XML import is not supported here.</p></div><label v-if="linkPreview&&['html','txt'].includes(linkPreview.detected_type)">Literature category at intake<select v-model="intakeLiteratureCategory"><option value="">Let extraction suggest a category</option><option v-for="value in literatureOptions" :key="value" :value="value">{{value.replaceAll('_',' ')}}</option></select></label><details class="optional-details"><summary>Add source details <span>Optional</span></summary><div class="metadata-grid"><label>Title<input v-model="linkTitle" maxlength="300" /></label><label>Author<input v-model="linkAuthor" maxlength="300" /></label><label>Edition or volume<input v-model="linkEdition" maxlength="300" /></label><label>Publication details<input v-model="linkPublication" maxlength="500" /></label><label>Repository or collection<input v-model="linkRepository" maxlength="300" /></label><label>Catalogue URL<input v-model="linkSourceURL" type="url" maxlength="1000" /></label><label class="full-width">Rights statement<textarea v-model="linkRights" rows="2" maxlength="2000" /></label></div></details><div class="intake-footer"><span>Import fetches the URL again and saves that snapshot.</span><button class="primary-button" :disabled="busy||!linkPDF.trim()||!!linkPreview&&!(['html','txt'].includes(linkPreview.detected_type)||linkPreview.can_import_pdf)">{{busy?'Importing…':linkPreview&&['html','txt'].includes(linkPreview.detected_type)?'Import document for review':'Import PDF & process'}}</button></div></form>
    </div>
    <div v-show="intakeMethod==='archive'" class="intake-body">
   <form v-if="session.role==='admin'" class="step" @submit.prevent="searchArchive(1)"><h2>Search source catalog</h2><p>Search Internet Archive by title or author. Inspect its catalogue record and PDF before downloading. Every imported book still needs page and rights review.</p><label>Book title or author <input v-model="archiveQuery" minlength="3" maxlength="120" placeholder="Boericke or Organon of Medicine" /></label><button :disabled="busy||archiveQuery.trim().length<3">Search books</button></form>
   <div v-if="session.role==='admin'&&(archiveResults.length||archiveTotal)" class="step"><h3>Internet Archive results</h3><p>{{archiveTotal}} matching records. Choose a record to see its available PDFs.</p><div class="archive-results"><article v-for="work in archiveResults" :key="work.identifier"><strong>{{work.title||work.identifier}}</strong><p>{{work.creator||'Author not listed'}} · {{work.year||'Year unknown'}}</p><p><a :href="work.record_url" target="_blank" rel="noreferrer">Open catalogue record</a> <button :disabled="busy" @click="inspectArchive(work.identifier)">View PDFs</button></p></article></div><div class="actions"><button :disabled="busy||archivePage<=1" @click="searchArchive(archivePage-1)">Previous</button><span>Page {{archivePage}}</span><button :disabled="busy||archivePage*12>=archiveTotal" @click="searchArchive(archivePage+1)">Next</button></div></div>
   <div v-if="session.role==='admin'&&archiveItem" id="archive-item" class="step"><h3>{{archiveItem.title||archiveItem.identifier}}</h3><p>{{archiveItem.creator||'Author not listed'}} · {{archiveItem.publication_info||archiveItem.year||'Date unknown'}}</p><p><a :href="archiveItem.record_url" target="_blank" rel="noreferrer">Review catalogue record</a></p><p>Recorded rights: {{archiveItem.rights||'No rights statement supplied; check the catalogue record before publishing.'}} <a v-if="archiveItem.license_url" :href="archiveItem.license_url" target="_blank" rel="noreferrer">Licence</a></p><p v-if="!archiveItem.pdfs.length">No public PDF under 250 MB was listed for this item. Try another record or use a permitted PDF link.</p><div v-for="file in archiveItem.pdfs" :key="file.url" class="archive-file"><span>{{file.name}} · {{(file.bytes/1048576).toFixed(1)}} MB · {{file.source||'PDF'}}</span><button :disabled="busy" @click="importArchivePDF(file)">Download for review</button></div></div>
   <p v-if="archiveNotice" class="good" role="status">{{archiveNotice}}</p>
    </div>
    <div v-show="intakeMethod==='doi'" class="intake-body">
   <form v-if="session.role==='admin'" class="step" @submit.prevent="addDOI"><h2>Add by DOI</h2><p>Look up a paper and save its reference. The result appears below. It can be cited in answers only after readable text is imported, checked, and published.</p><label>DOI or doi.org link <input v-model="doiInput" placeholder="10.1186/s13643-023-02313-2" required /></label><button :disabled="busy||!doiInput.trim()">{{busy?'Looking up DOI…':'Look up DOI'}}</button><p v-if="doiNotice" class="good" role="status">{{doiNotice}}</p></form>
    </div>
   </section>
   <SourceLibrary :sources="librarySources" :can-add="session.role==='admin'" :loading="sourcesLoading" @open="chooseSource" @add="openIntake" />
   <details v-if="doiReferences.length" class="saved-references" :open="intakeMethod==='doi'&&intakeOpen"><summary>Saved paper references <span>{{doiReferences.length}}</span></summary><p class="input-hint">A saved reference becomes answer evidence only after its full text is imported, reviewed and prepared.</p>
   <div v-if="doiReferences.length" class="step"><h2>DOI references</h2><article v-for="item in doiReferences" :key="item.id" class="doi-reference" :class="{highlighted:item.id===lastDOIID}"><strong>{{item.title}}</strong><p>{{item.authors}} · {{item.publication_year||'Year unknown'}} · {{item.publisher}}</p><p><a :href="item.doi_url" target="_blank" rel="noreferrer">{{item.doi}}</a> · {{item.metadata_provider}} metadata</p><p v-if="item.retraction_notice_url" class="error">Retracted article. Excluded from answers. <a :href="item.retraction_notice_url" target="_blank" rel="noreferrer">Read publisher notice</a>.</p><p v-else-if="item.source_id" class="good">PDF imported. Open the imported document in the source library to check its progress.</p><template v-else-if="!item.retraction_notice_url"><p>Reference saved. This paper is not yet used in answers.</p><button v-if="session.role==='admin'&&item.pdf_available" :disabled="busy" @click="importDOI(item.id)">Import eligible PDF for analysis</button><p v-else>No eligible direct PDF link was found. Use Add source → Upload PDF if you have a permitted copy.</p></template><p v-if="item.license_url&&item.pdf_available"><a :href="item.license_url" target="_blank" rel="noreferrer">Check recorded licence before publishing</a></p><p><button v-if="session.role==='admin'" :disabled="busy" @click="deleteDOI(item)">Delete reference</button></p></article></div>
   </details>
  </section>

  <section v-else-if="tab==='review'" class="source-review-workspace">
   <button class="back-link" @click="tab='sources'">← Back to source library</button>
   <p class="eyebrow">SOURCE REVIEW</p><h1>Prepare a source for research</h1>
   <p v-if="!active">Choose a book in Sources to see what needs checking.</p>
   <template v-else>
   <h2>{{active.title}}</h2>
   <div v-if="detail" class="step"><h3>Literature category</h3><p>Current: {{detail.literature_categories.join(', ').replaceAll('_',' ')}} · {{detail.literature_category_origin}}. Evidence category: {{detail.evidence_category.replaceAll('_',' ')}}. Format: {{detail.document_format.toUpperCase()}}.</p><p v-if="detail.classification_state">Automatically detected ({{detail.classifier_version}}): {{detail.classification_state}} · {{detail.classification_reason}}</p><p v-else>No content classification is recorded for this revision. Existing sources remain unclassified until reviewed.</p><div v-if="session.role==='admin'&&(active.status==='review'||active.status==='published')"><p>Choose one or more literature categories. Unclassified is a standalone choice.</p><div class="actions"><label v-for="value in literatureOptions" :key="value"><input type="checkbox" :checked="reviewLiteratureCategories.includes(value)" @change="toggleReviewCategory(value)" /> {{value.replaceAll('_',' ')}}</label></div><label>Evidence category<select v-model="reviewEvidenceCategory"><option v-for="value in evidenceOptions" :key="value" :value="value">{{value.replaceAll('_',' ')}}</option></select></label><label>Reason<textarea v-model="categoryReason" rows="2" placeholder="What in the content supports this choice?" /></label><div class="actions"><button :disabled="busy" @click="saveLiteratureCategories('manual')">Save reviewed categories</button><button v-if="detail.classification_state==='suggested'" :disabled="busy" @click="saveLiteratureCategories('accepted_suggestion')">Accept detected {{detail.suggested_categories.join(', ').replaceAll('_',' ')}}</button><button :disabled="busy" @click="retryClassification">Retry detection</button></div></div></div>
   <template v-if="detail?.document_format&&detail.document_format!=='pdf'">
    <div class="step"><h3>{{detail.document_format.toUpperCase()}} document · {{active.status}}</h3><p>Asset {{detail.source_asset_id}} · revision {{detail.processing_revision_id}} · {{detail.document_blocks_reviewed}} of {{detail.document_blocks}} blocks reviewed.</p><p v-if="detail.final_url">Requested {{detail.requested_url}}<br />Saved from {{detail.final_url}}</p><p>{{detail.transport==='http'?'Unencrypted HTTP acquisition':detail.transport==='https'?'HTTPS acquisition':'Uploaded file'}} · {{detail.acquired_at}}</p><p v-if="active.status==='queued'||active.status==='processing'">Extraction continues in the background.</p><p v-if="active.status==='failed'" class="error">{{active.error}}</p></div>
    <details v-if="session.role==='admin'&&active.status==='review'" class="step" open><summary>Source details</summary><form class="metadata-grid" @submit.prevent="saveMetadata"><label>Title<input v-model="metadata.title" required /></label><label>Author<input v-model="metadata.author" required /></label><label>Edition<input v-model="metadata.edition" /></label><label>Publication details<input v-model="metadata.publication_info" /></label><label>Repository<input v-model="metadata.repository" /></label><label>Source URL<input v-model="metadata.source_url" type="url" /></label><label>Rights statement<textarea v-model="metadata.rights_statement" rows="2" /></label><button :disabled="busy||!metadata.title.trim()||!metadata.author.trim()">Save details</button></form></details>
    <div v-if="active.status==='review'" class="step"><h3>Review extracted blocks</h3><p>Original and extracted text are displayed as inert text. Check headings, tables and uncertain boundaries against the saved original.</p><article v-for="block in documentBlocks" :key="block.id" class="step"><h4>{{block.block_index+1}}. {{block.kind}} · {{block.section_key}} · {{block.review_status}}</h4><p v-if="block.heading">Under {{block.heading}}</p><p v-for="warning in block.warnings" :key="warning" class="error">{{warning}}</p><div class="compare"><div><b>Original extraction</b><pre>{{block.original_text}}</pre></div><div><b>Reviewed text</b><textarea v-model="blockTexts[block.id]" rows="5" :disabled="session.role!=='admin'" /></div></div><label>Review note<textarea v-model="blockNotes[block.id]" rows="2" :disabled="session.role!=='admin'" /></label><div v-if="session.role==='admin'" class="actions"><button :disabled="busy" @click="reviewDocumentBlock(block,'accepted')">Accept</button><button :disabled="busy||!blockNotes[block.id]?.trim()||!blockTexts[block.id]?.trim()" @click="reviewDocumentBlock(block,'corrected')">Save correction as new revision</button><button :disabled="busy||!blockNotes[block.id]?.trim()" @click="reviewDocumentBlock(block,'excluded')">Exclude</button></div><details v-if="session.role==='admin'"><summary>Section literature category {{block.category_override?'(reviewed override)':'(inherits source)'}}</summary><div class="actions"><label v-for="value in literatureOptions" :key="value"><input type="checkbox" :checked="(sectionCategories[block.id]||[]).includes(value)" @change="toggleSectionCategory(block.id,value)" /> {{value.replaceAll('_',' ')}}</label></div><label>Reason<textarea v-model="sectionCategoryReasons[block.id]" rows="2" /></label><button :disabled="busy" @click="saveSectionCategories(block)">Save section category</button></details></article></div>
    <div v-if="active.status==='review'" class="step"><h3>Rights decision"</h3><p v-if="detail.rights_statement">Recorded statement: {{detail.rights_statement}}</p><p v-if="detail.final_url"><a :href="detail.final_url" target="_blank" rel="noreferrer">Check original URL</a></p><label>Decision<select v-model="rightsDecision"><option value="" disabled>Choose</option><option value="allowed">Allowed</option><option value="denied">Denied</option></select></label><label>Reason<textarea v-model="rightsNote" rows="2" /></label><button :disabled="busy||!rightsDecision||!rightsNote.trim()" @click="saveRights">Save decision</button><p v-if="detail.rights_status==='allowed'" class="good">Allowed decision saved for this revision.</p></div>
    <div v-if="active.status==='review'" class="step"><h3>Publish and prepare passages</h3><ul v-if="publicationNeeds.length"><li v-for="need in publicationNeeds" :key="need">{{need}}</li></ul><button v-if="session.role==='admin'" :disabled="busy||!canPublish" @click="publish">Make available for questions</button></div>
    <div v-if="active.status==='published'" class="step"><p v-if="index?.status==='ready'&&index.matches_config" class="good">Ready to ask. <button @click="askAboutSource">Ask about this source</button></p><p v-else>Preparing passages: {{index?.completed||0}} of {{index?.total||0}}. <button v-if="session.role==='admin'&&index?.status==='failed'" @click="retryIndex">Retry</button></p><p>Saved citations use this original snapshot even if the remote page changes.</p><button v-if="session.role==='admin'&&detail?.requested_url&&!activeCandidate&&!detail.superseded" :disabled="busy" @click="reimportDocument">Fetch current URL as a review candidate</button><p v-if="activeCandidate">A candidate is being reviewed; this published snapshot remains available.</p></div>
   </template>
   <template v-else>
   <div v-if="session.role==='admin'&&(active.status==='published'||active.status==='disabled')" class="step"><h3>Source availability</h3><p v-if="active.status==='disabled'">This source is excluded from new answers and its citations are unavailable until restored.</p><p v-else>Disabling a source removes it from new answers and citation access immediately. You can restore it after review.</p><label>Reason <textarea v-model="sourceAccessReason" rows="2" minlength="8" maxlength="1000" placeholder="Record why availability is changing" /></label><button :disabled="busy||sourceAccessReason.trim().length<8" @click="setSourceAccess(active.status==='disabled')">{{active.status==='disabled'?'Restore source':'Disable source'}}</button></div>
	<p v-if="detail?.retraction_notice_url" class="error" role="alert">This article was retracted. Keep it for a record of the publication, but do not use it as evidence for clinical answers. <a :href="detail.retraction_notice_url" target="_blank" rel="noreferrer">Read the publisher's retraction notice</a>.</p>
    <ol class="journey" aria-label="Source steps"><li class="done">Add source</li><li :class="{done:active.pages_read===active.pages_total}">Read pages</li><li :class="{current:active.status==='review',done:active.status==='published'}">Check pages</li><li :class="{current:canPublish,done:active.status==='published'}">Make available</li><li :class="{current:active.status==='published'&&index?.status!=='ready',done:index?.status==='ready'}">Prepare passages</li><li :class="{done:index?.status==='ready'}">Ask</li></ol>
    <p v-if="active.status==='queued'||active.status==='processing'">Reading scanned pages: {{active.pages_read}} of {{active.pages_total}}. You can leave this screen and return.</p>
    <p v-else-if="active.status==='failed'" class="error">We couldn’t finish reading this book. Your original PDF is saved. {{active.error}}</p>
    <p v-else-if="active.status==='review'&&!detail?.retraction_notice_url">The system checks the text against the scans. Only uncertain pages need your decision.</p>
    <div v-else-if="active.status==='published'" class="step"><p v-if="index?.status==='ready'&&index.matches_config" class="good">Ready to ask. {{index.completed}} of {{index.total}} passages prepared with {{index.model}}. <button @click="askAboutSource">Ask about this source</button></p><p v-else-if="index?.status==='ready'" class="error">This source was prepared with {{index.model}}, which differs from the configured embedding model. <button v-if="session.role==='admin'" :disabled="busy" @click="retryIndex">Reindex with current model</button></p><p v-else-if="index?.status==='pending'||index?.status==='running'">Preparing passages for search: {{index.completed}} of {{index.total}}. This continues in the background.</p><p v-else-if="index?.status==='failed'" class="error">Preparation stopped after {{index.completed}} of {{index.total}} passages: {{index.error}} <button v-if="session.role==='admin'" @click="retryIndex">Try again</button></p><p v-else>Passages have not been prepared. <button v-if="session.role==='admin'" @click="retryIndex">Prepare now</button></p></div>

    <div v-if="detail?.supersedes_source_id" class="step"><h3>Candidate revision</h3><p>This copy is being processed from the saved PDF. The current publication remains available for questions until this copy passes review and its passages are READY.</p><button v-if="sources.find(source=>source.id===detail?.supersedes_source_id)" @click="chooseSource(sources.find(source=>source.id===detail?.supersedes_source_id)!)">View current source</button></div>
    <div v-if="active.status==='published'" class="step"><h3>Processing revision</h3><p v-if="detail?.superseded">This source has been replaced for new searches. Its saved citations still open the original pages.</p><p v-else-if="activeCandidate">A candidate revision is in progress. The current source stays available until the candidate is ready.</p><p v-else>Create a new candidate from this saved PDF when page extraction or metadata needs a fresh review. The current answer evidence stays available while it runs.</p><button v-if="session.role==='admin'&&!detail?.superseded&&!activeCandidate" :disabled="busy" @click="reprocessSource">Reprocess saved PDF</button></div>
    <details v-if="detail?.processing_revision_id" class="step"><summary>Source provenance</summary><p>Edition {{detail.edition_id}}<br />Acquisition record {{detail.source_record_id}}<br />PDF asset {{detail.source_asset_id}}<br />Processing revision {{detail.processing_revision_id}}<br /><template v-if="detail.rights_decision_id">Rights decision {{detail.rights_decision_id}}<br /></template>PDF SHA-256 {{detail.pdf_sha256}}</p></details>
    <template v-if="reviewReady">
     <details v-if="session.role==='admin'" class="step source-metadata"><summary>Source details <span>Check the detected title, author and edition</span></summary><form class="metadata-grid" @submit.prevent="saveMetadata"><label>Title <input v-model="metadata.title" required /></label><label>Author <input v-model="metadata.author" required /></label><label>Edition <input v-model="metadata.edition" /></label><label>Publication details <input v-model="metadata.publication_info" /></label><label>Repository or collection <input v-model="metadata.repository" /></label><label>Source URL <input v-model="metadata.source_url" type="url" /></label><label>Rights statement <textarea v-model="metadata.rights_statement" rows="2" /></label><button :disabled="busy||!metadata.title.trim()||!metadata.author.trim()">Save details</button></form></details>
     <div class="step"><h3>1. Check the pages</h3><p>Clear blank scans are omitted from answers. The system compares stored text with a fresh OCR pass; pages with suspected problems stay in your review list. Every original scan remains in the PDF.</p><p v-if="detail?.triage_status==='queued'||detail?.triage_status==='running'">Checking scans without text: {{detail?.triage_completed}} of {{detail?.triage_total}}.</p><p v-if="detail?.triage_status==='failed'" class="error">Blank scan check stopped: {{detail?.triage_error}}</p><p v-if="detail?.text_qa_status==='queued'||detail?.text_qa_status==='running'">Checking text: {{detail?.text_pages_checked}} of {{detail?.text_pages_total}} pages. You can leave and return.</p><p v-if="detail?.text_qa_status==='failed'" class="error">Text check stopped: {{detail?.text_qa_error}}</p><p><strong>{{detail?.auto_blank_pages||0}} automatically marked blank · {{detail?.suspect_text_pages||0}} text pages need a look · {{detail?.unclassified_pages||0}} other scans need a decision · {{detail?.missing_text_pages||0}} need text repair</strong></p></div>
     <div class="actions"><button :class="{selected:needsAttention}" @click="needsAttention=true;selectedPage=null;loadPages()">Needs your decision</button><button :class="{selected:!needsAttention}" @click="needsAttention=false;selectedPage=null;loadPages()">Browse scans</button><template v-if="!needsAttention"><button :disabled="pageOffset===0" @click="movePages(-30)">Previous</button><span>Scans {{pageOffset+1}}–{{Math.min(pageOffset+30,active.pages_total)}}</span><button :disabled="pageOffset+30>=active.pages_total" @click="movePages(30)">Next</button></template><label>Go to scan <input v-model.number="scanNumber" type="number" min="1" :max="active.pages_total" /></label><button @click="jumpToScan">Go</button><label v-if="!needsAttention" class="check"><input v-model="showOmitted" type="checkbox" @change="loadPages" /> Show omitted scans</label></div>
     <p v-if="needsAttention&&!pages.length" class="good">No uncertain scans are waiting for a decision.</p><div class="page-list"><button v-for="p in pages" :key="p.id" :class="{selected:selectedPage?.id===p.id}" @click="selectPage(p)">Scan {{p.scan_page_index+1}} <small>{{p.printed_label?'page '+p.printed_label:'page unlabeled'}} · {{pageState(p)}}</small></button></div>
     <div v-if="selectedPage" :key="selectedPage.id" class="page-review"><h3>Scan {{selectedPage.scan_page_index+1}} <small>{{selectedPage.printed_label?'· printed page '+selectedPage.printed_label:''}}</small></h3><p v-if="selectedPage.triage_reason">{{selectedPage.triage_reason}}</p><p v-if="selectedPage.text_qa_status==='suspect'">{{selectedPage.text_qa_reason}}</p><p v-if="selectedPage.page_kind==='unclassified'&&selectedPage.review_note">Earlier note: {{selectedPage.review_note}}. Check this scan again before saving a decision.</p><div class="compare"><div><a :href="selectedPage.image_url" target="_blank" rel="noreferrer">Open full scan in a new tab</a><img :src="selectedPage.image_url" :alt="`Original scan ${selectedPage.scan_page_index+1}`" /></div><div><b>Stored text</b><pre>{{selectedPage.text||'No text found. Check whether this scan is blank, illustrated, or missing readable text.'}}</pre></div></div><label class="note-label">Printed page label <input v-model="printedLabel" placeholder="e.g. xiv or 12" /></label><button :disabled="busy" @click="savePrintedLabel">Save page label</button><label class="note-label">What does this scan contain? <select v-model="pageOutcome"><option value="" disabled>Choose an outcome</option><option value="text">Text page — keep searchable</option><option value="blank">Blank page — omit from answers</option><option value="illustration">Cover, diagram or illustration — keep visible</option><option value="book_info">Library or book information — omit from answers</option><option value="missing_text">Printed text is missing or incomplete — needs repair</option></select></label><label v-if="pageNoteRequired" class="note-label">What did you check? <textarea v-model="pageNote" rows="2" placeholder="Describe what you see and any missing lines or page-number differences" /></label><button :disabled="busy||!pageOutcome||(pageNoteRequired&&!pageNote.trim())" @click="reviewPage">Save this page review</button></div>

     <div class="step"><h3>2. Check permission to use the scan</h3><p v-if="detail?.rights_mark">Repository mark: “{{detail.rights_mark}}”. <a v-if="detail.rights_evidence_url" :href="detail.rights_evidence_url" target="_blank" rel="noreferrer">Read the rights statement</a>.</p><p v-if="detail?.rights_statement">Recorded rights statement: {{detail.rights_statement}}</p><p v-if="detail?.source_url"><a :href="detail.source_url" target="_blank" rel="noreferrer">Open catalogue record</a></p><p v-if="detail?.pdf_origin_url"><a :href="detail.pdf_origin_url" target="_blank" rel="noreferrer">Open original PDF link</a></p><p>Check the original source and record whether this local research use is permitted.</p><label>Your decision <select v-model="rightsDecision"><option value="" disabled>Choose after checking</option><option value="allowed">Allowed for this use</option><option value="denied">Not allowed</option></select></label><label class="note-label">Reason for the decision <textarea v-model="rightsNote" rows="2" placeholder="Record the evidence and any limits" /></label><button :disabled="busy||!rightsDecision||!rightsNote.trim()" @click="saveRights">Save decision</button><p v-if="detail?.rights_status==='allowed'" class="good">Permission decision saved as allowed.</p></div>
     <div class="step"><h3>3. Make the book available</h3><template v-if="!canPublish"><p>Complete these items to make this source searchable:</p><ul><li v-for="need in publicationNeeds" :key="need">{{need}}</li></ul></template><p v-else>Required checks are recorded. Publishing will prepare this source for search.</p><button v-if="session.role==='admin'" :disabled="busy||!canPublish" @click="publish">Make available for questions</button></div>
    </template>
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
   <section v-if="session.role==='admin'&&evaluationReport" class="step evaluation-report"><div class="sectionhead"><h2>Evaluation report</h2><div class="actions"><button @click="downloadEvaluation">Download JSON</button><button @click="evaluationReport=null">Close</button></div></div><p><strong>Job {{evaluationReport.job_id}}</strong> · {{evaluationReport.owner}} · {{evaluationReport.job_status}} · {{evaluationReport.research_mode}} · {{evaluationReport.attempts}} attempt(s)</p><p>Submitted {{new Date(evaluationReport.created_at).toLocaleString()}}<span v-if="evaluationReport.finished_at"> · Finished {{new Date(evaluationReport.finished_at).toLocaleString()}}</span></p><h3>User input</h3><pre>{{evaluationReport.question_raw}}</pre><h3>System answer</h3><p>{{evaluationReport.answer||'No answer saved yet.'}}</p><p>Status: {{evaluationReport.answer_status||evaluationReport.stage}}</p><h3>RAG metrics</h3><p v-if="evaluationReport.answer_status">{{evaluationReport.metrics.search_count}} searches · {{evaluationReport.metrics.retrieved_passages_across_searches}} retrieved across searches · {{evaluationReport.metrics.selected_candidate_count}} selected candidates · {{evaluationReport.metrics.cited_passage_count}} cited · {{Math.round(evaluationReport.metrics.citation_coverage*100)}}% candidate citation coverage · {{evaluationReport.metrics.supported_claim_count}} supported claims · {{evaluationReport.metrics.rejected_claim_count}} rejected claims</p><p v-else>Metrics will appear when an answer is saved.</p><p v-if="evaluationReport.answer_status">Top retrieval score {{evaluationReport.metrics.top_retrieval_score.toFixed(3)}} · Mean {{evaluationReport.metrics.mean_retrieval_score.toFixed(3)}}</p><p>Answer model: {{evaluationReport.answer_model||'not recorded'}} {{evaluationReport.answer_revision}} · Embedding model: {{evaluationReport.embedding_model||'not recorded'}} {{evaluationReport.embedding_revision}} · Prompt: {{evaluationReport.prompt_revision||'not recorded'}}</p><details><summary>Model calls ({{evaluationReport.model_usage_summary.call_count}})</summary><p>Known provider estimated cost: ${{evaluationReport.model_usage_summary.known_estimated_cost_usd.toFixed(6)}} · {{evaluationReport.model_usage_summary.unknown_cost_call_count}} call(s) with unknown cost</p><p v-for="(call,index) in evaluationReport.model_calls" :key="index">{{call.kind}} · {{call.provider}} · {{call.requested_model}} · {{call.outcome}} · {{call.total_tokens??'unknown'}} tokens · {{call.estimated_cost_usd===null?'cost unknown':'$'+call.estimated_cost_usd.toFixed(6)}} · {{call.duration_ms}} ms</p></details><details><summary>Searches and retrieved passages</summary><p v-for="search in evaluationReport.searches" :key="search.ordinal">{{search.ordinal}}. {{search.query}} · {{search.scope}} · {{search.candidate_count}} found</p><article v-for="candidate in evaluationReport.candidates" :key="candidate.chunk_id"><p><strong>{{candidate.rank}}. {{candidate.title}}</strong> by {{candidate.author}} · scan {{candidate.scan}} · score {{candidate.score.toFixed(3)}} · {{candidate.cited?'cited':'not cited'}}</p><blockquote>{{candidate.preview}}</blockquote></article></details><details><summary>Claim checks ({{evaluationReport.claims.length}})</summary><p v-for="claim in evaluationReport.claims" :key="claim.ordinal">{{claim.ordinal}}. {{claim.decision}} · {{claim.claim}}</p></details><details open><summary>Technical log ({{evaluationReport.logs.length}})</summary><p v-for="(entry,index) in evaluationReport.logs" :key="index">{{new Date(entry.created_at).toLocaleString()}} · attempt {{entry.attempt}} · {{entry.stage}}<span v-if="entry.duration_ms!==null"> · {{entry.duration_ms}} ms</span><br><code>{{JSON.stringify(entry.detail)}}</code></p></details></section>
   <div class="actions"><button :disabled="activityPage===1" @click="changeActivityPage(-1)">Previous</button><span>Page {{activityPage}}</span><button :disabled="answerJobs.length<20&&sourceJobs.length<20" @click="changeActivityPage(1)">Next</button></div>
  </section>
  <section v-else>
   <h1>Ask your sources</h1>
   <p>Searches {{readyCount}} prepared {{readyCount===1?'source':'sources'}}. Ask about a source, what an author reports, or how authors compare. Open each citation to check the passage against its original scan. Historical claims are not modern clinical evidence.</p>
   <div v-if="readyCount===0" class="empty">Prepare a source before asking questions. <button @click="tab='sources'">View sources</button></div>
   <form v-else @submit.prevent="ask">
    <label for="question">Your question</label><textarea id="question" v-model="question" rows="3" maxlength="1000" placeholder="How do Nash and Farrington describe Lachesis?"/>
    <p v-if="question.trim()&&!validQuestion" class="input-hint">Add a topic or remedy name with at least 3 letters, such as “mood” or “Nux”.</p>
    <fieldset class="source-picker"><legend>Literature categories</legend><p>Choose none to search all eligible passages. Selected categories intersect with the sources below.</p><div class="source-options"><label v-for="value in literatureOptions" :key="value"><input type="checkbox" :checked="selectedLiteratureCategories.includes(value)" @change="toggleAskCategory(value)" /> {{value.replaceAll('_',' ')}}</label></div></fieldset>
    <fieldset class="source-picker"><legend>Sources to search</legend><label><input type="radio" name="source-mode" :checked="sourceSelectionMode==='all'" @change="setSourceSelectionMode('all')" /> All {{readyCount}} prepared sources</label><label><input type="radio" name="source-mode" :checked="sourceSelectionMode==='selected'" @change="setSourceSelectionMode('selected')" /> Choose sources</label><div v-if="sourceSelectionMode==='selected'" class="source-options"><label v-for="source in readySources" :key="source.id"><input v-model="selectedSourceIds" type="checkbox" :value="source.id" /><span><strong>{{source.title}}</strong><br />{{source.author}}</span></label><p>{{selectedSourceIds.length}} selected</p><p v-if="!selectionValid&&!asksAboutSelectedSource" class="input-hint">Select at least one prepared source. Refresh if a source is no longer ready.</p></div><p v-if="asksAboutSelectedSource&&!selectionValid" class="input-hint">Your question refers to one selected source. Choose sources and select exactly one paper.</p></fieldset>
    <label for="research-mode">Research depth</label><select id="research-mode" v-model="researchMode"><option value="quick">Quick answer · one search per source</option><option value="deep">Detailed research · three focused searches</option></select>
    <button :disabled="busy||!validQuestion||!selectionValid">{{busy?'Saving question…':researchMode==='deep'?'Research in detail':'Ask'}}</button>
   </form>
   <div v-if="!answer&&questionJob" class="step" role="status"><p><strong>Your question:</strong> {{questionJob.stage}}.</p><p v-if="questionJob.status==='working'||questionJob.status==='waiting'||questionJob.status==='retrying'">Your question is saved. The answer will appear here when it is ready.</p><p v-if="questionJob.status==='failed'" class="error">{{questionJob.error_message||'The answer could not be completed.'}}</p><button v-if="questionJob.answer_id" @click="openAnswerJob(questionJob)">Open answer</button><button v-else-if="questionJob.status==='failed'&&questionJob.error_code==='source_unavailable'" @click="reviseAnswerJob(questionJob)">Change sources</button><button v-else-if="questionJob.status==='failed'" :disabled="busy" @click="retryAnswerJob(questionJob)">Try again</button></div>
   <article v-if="answer" class="answer">
   <h2>{{answerStatus==='insufficient_evidence'?'Not enough support yet':answerStatus==='partial'?'Partial answer':sections.length?'Research answer':'Answer from your sources'}}</h2>
    <p>Literature scope: {{answerCategoryScope.length?answerCategoryScope.join(', ').replaceAll('_',' '):'all eligible categories'}}. One-page imports do not establish coverage of an entire work.</p>
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
    <section v-if="citations.length" class="source-refs"><h3>Open cited passages</h3><button v-for="c in citations" :key="c.id" @click="showCitation(c.id)">{{c.label}} · {{c.author}} · {{passageLocation(c)}} · {{c.literature_categories?.join(', ')?.replaceAll('_',' ')||'unclassified'}}</button></section>
    <section v-if="answerStatus!=='insufficient_evidence'" class="source-refs"><h3>Scope and next checks</h3><p>This answer uses prepared sources. Open the cited scans to check context. Historical descriptions alone do not establish present-day clinical evidence.</p></section>
   </article>
  </section>
  <section v-if="tab==='ask'&&evidence.length" class="evidence-section"><h2>Evidence passages</h2><p v-if="citedEvidence.length">These passages support the displayed answer. Open a scan to check its context.</p><div v-if="citedEvidence.length" class="evidence-grid"><article v-for="item in citedEvidence" :key="item.rank" class="evidence-card"><div class="evidence-head"><b>{{item.author}}</b><span>Cited</span></div><p>{{item.title}} · {{passageLocation(item)}} · {{item.literature_categories?.join(', ')?.replaceAll('_',' ')||'unclassified'}} · {{item.evidence_category||'unknown evidence category'}}</p><blockquote>{{item.preview}}</blockquote><a v-if="item.image_url" :href="item.image_url" target="_blank" rel="noreferrer">Open original scan {{item.scan_position}}</a><span v-else>Saved document section</span></article></div><details v-if="uncitedEvidence.length"><summary>Show {{uncitedEvidence.length}} retrieved {{uncitedEvidence.length===1?'passage':'passages'}} not used in the answer</summary><p>Search retrieved these candidates, but they did not support the displayed answer. Some may be unrelated to your question.</p><div class="evidence-grid"><article v-for="item in uncitedEvidence" :key="item.rank" class="evidence-card"><div class="evidence-head"><b>{{item.author}}</b><span>Not cited</span></div><p>{{item.title}} · {{passageLocation(item)}} · {{item.literature_categories?.join(', ')?.replaceAll('_',' ')||'unclassified'}} · {{item.evidence_category||'unknown evidence category'}}</p><blockquote>{{item.preview}}</blockquote><a v-if="item.image_url" :href="item.image_url" target="_blank" rel="noreferrer">Open original scan {{item.scan_position}}</a><span v-else>Saved document section</span></article></div></details></section>
  <aside v-if="citation" class="drawer" aria-label="Source reference"><button class="close" @click="citation=null">Close</button><h2>{{citation.title}}</h2><p>{{citation.author}} · {{passageLocation(citation)}}</p><blockquote>{{citation.passage}}</blockquote><div v-if="citation.reader_text"><p>{{citation.format?.toUpperCase()}} · {{citation.section_key}} · {{citation.literature_categories?.join(', ')?.replaceAll('_',' ')||'unclassified'}} · {{citation.evidence_category||'unknown evidence category'}}</p><pre>{{citationPart(citation,0,citation.start_character)}}<mark>{{citationPart(citation,citation.start_character||0,citation.end_character)}}</mark>{{citationPart(citation,citation.end_character||0)}}</pre><p v-if="citation.original_text!==citation.reader_text">Original extracted text: {{citation.original_text}}</p><p><a v-if="citation.document_url" :href="citation.document_url" target="_blank" rel="noreferrer">Download saved original as inert text</a></p><p v-if="citation.original_url"><a :href="citation.original_url" target="_blank" rel="noreferrer">Original URL at acquisition</a></p></div><template v-else><img v-if="citation.image_url" class="citation-scan" :src="citation.image_url" :alt="`Original scan ${citation.scan_position}`" /><p><a v-if="citation.pdf_url" :href="citation.pdf_url" target="_blank" rel="noreferrer">Open saved PDF at this page</a></p><p v-if="citation.source_url"><a :href="citation.source_url" target="_blank" rel="noreferrer">Open source record or DOI</a></p><a v-if="citation.image_url" :href="citation.image_url" target="_blank" rel="noreferrer">Open full scan</a></template></aside>
  </template>
 </main>
</template>
