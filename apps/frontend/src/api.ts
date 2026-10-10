export type Source={collection_id?:string|null,collection_title?:string,collection_eligible?:boolean,removed:boolean,deletion_blocker:string,id:string,title:string,author:string,status:string,rights_status:string,document_format:'pdf'|'html'|'txt',literature_categories:string[],evidence_category:string,pages_read:number,pages_total:number,error:string,supersedes_source_id:string|null,superseded:boolean}
export type Page={text_revision:number,id:string,pdf_page_index:number,scan_page_index:number,printed_label:string,review_status:string,page_kind:string,review_note:string,triage_reason:string,text_qa_status:string,text_qa_reason:string,text:string,image_url:string}
export type Citation={id:string,passage:string,title:string,author:string,printed_page?:string,scan_position?:number,pdf_url?:string,image_url?:string,source_url?:string,format?:string,section_key?:string,reader_text?:string,original_text?:string,start_character?:number,end_character?:number,original_url?:string,document_url?:string,reader_url?:string,literature_categories?:string[],evidence_category?:string}
export class ApiError extends Error{constructor(message:string,public status:number){super(message)}}
export async function api<T>(path:string,init:RequestInit={}):Promise<T>{
 let res:Response
 try{res=await fetch('/api/v1'+path,{...init,headers:{...(init.body instanceof FormData?{}:{'Content-Type':'application/json'}),...init.headers}})}
 catch{throw new Error('The service is unavailable. Try again when it is running.')}
 let data:unknown
 try{data=await res.json()}catch{throw new Error(`The API returned HTTP ${res.status} for ${path}. Check that the current API version is running.`)}
 if(!res.ok)throw new ApiError((data as {error?:string}).error||`Request failed (HTTP ${res.status}) for ${path}`,res.status)
 return data as T
}
