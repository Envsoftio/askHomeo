<script setup lang="ts">
import {computed, ref, watch} from 'vue'
import type {Source} from '../api'

export type LibraryGroup = 'ready' | 'processing' | 'attention' | 'unavailable'
export type LibrarySource = Source & {group: LibraryGroup; state: string}
const props = defineProps<{sources: LibrarySource[]; canAdd: boolean; loading: boolean}>()
const emit = defineEmits<{open: [source: Source]; add: []}>()
const search = ref('')
const filter = ref<LibraryGroup | 'all'>('all')
const sort = ref('recent')
const category = ref('all')
const categories=['materia_medica','repertory','organon_philosophy','therapeutics','provings','clinical_cases','research','other','unclassified']
const page = ref(1)
const filters: {id: LibraryGroup | 'all'; label: string}[] = [
  {id: 'all', label: 'All sources'}, {id: 'attention', label: 'Needs attention'},
  {id: 'processing', label: 'Processing'}, {id: 'ready', label: 'Ready to ask'},
  {id: 'unavailable', label: 'Unavailable'},
]
const count = (id: string) => props.sources.filter(s => id === 'all' || s.group === id).length
const filtered = computed(() => {
  const query = search.value.trim().toLocaleLowerCase()
  const rows = props.sources.filter(s => (filter.value === 'all' || s.group === filter.value) && (category.value==='all'||s.literature_categories.includes(category.value))
    && (!query || `${s.title} ${s.author}`.toLocaleLowerCase().includes(query)))
  if (sort.value === 'title') rows.sort((a,b) => a.title.localeCompare(b.title))
  if (sort.value === 'author') rows.sort((a,b) => a.author.localeCompare(b.author))
  return rows
})
const pageCount = computed(() => Math.max(1, Math.ceil(filtered.value.length / 12)))
const visible = computed(() => filtered.value.slice((page.value - 1) * 12, page.value * 12))
watch([search, filter, sort,category], () => {page.value = 1})
watch(pageCount, total => {page.value = Math.min(page.value, total)})
function reset() {search.value = ''; filter.value = 'all';category.value='all'}
</script>

<template>
  <section class="library" aria-labelledby="library-title" :aria-busy="loading">
    <div class="library-heading"><div><h2 id="library-title">Source library</h2><p>Original documents and their progress toward becoming searchable.</p></div><span class="total">{{sources.length}} sources</span></div>
    <div class="library-filters" aria-label="Filter sources by status">
      <button v-for="item in filters" :key="item.id" :aria-pressed="filter===item.id" @click="filter=item.id">{{item.label}} <span>{{count(item.id)}}</span></button>
    </div>
    <div class="library-toolbar">
      <label class="library-search">Search library<input v-model="search" type="search" placeholder="Search by title or author" /></label>
      <label>Literature<select v-model="category"><option value="all">All categories</option><option v-for="value in categories" :key="value" :value="value">{{value.replaceAll('_',' ')}}</option></select></label>
      <label>Sort by<select v-model="sort"><option value="recent">Recently added</option><option value="title">Title A–Z</option><option value="author">Author A–Z</option></select></label>
    </div>
    <p v-if="sources.length>=50" class="listing-limit">Showing the 50 most recently added sources. Search and status filters apply to this list.</p>
    <p v-if="loading&&!sources.length" class="library-empty" role="status">Loading your source library…</p>
    <div v-else-if="!sources.length" class="library-empty"><span class="empty-symbol" aria-hidden="true">＋</span><h3>Build your research library</h3><p>Add a book or paper. We’ll read the pages and show you what needs checking.</p><button v-if="canAdd" class="primary-button" @click="emit('add')">Add your first source</button><p v-else>An administrator can add sources to the shared library.</p></div>
    <div v-else-if="!filtered.length" class="library-empty"><h3>No matching sources</h3><p>Try another title, author or status.</p><button @click="reset">Clear filters</button></div>
    <template v-else>
      <div class="library-columns" aria-hidden="true"><span>Document</span><span>Status</span><span>Next step</span></div>
      <button v-for="source in visible" :key="source.id" class="library-row" @click="emit('open',source)">
        <span class="document-info"><span class="document-icon" aria-hidden="true">▤</span><span><strong>{{source.title}}</strong><small>{{source.author||'Author not recorded'}} · {{source.document_format.toUpperCase()}}<template v-if="source.document_format==='pdf'&&source.pages_total"> · {{source.pages_total}} pages</template></small><small>{{source.literature_categories.join(', ').replaceAll('_',' ')}} · {{source.evidence_category.replaceAll('_',' ')}}</small><small v-if="source.supersedes_source_id">Reprocessed edition</small></span></span>
        <span class="document-status"><span class="status-badge" :class="source.group">{{source.state}}</span><progress v-if="source.status==='processing'&&source.pages_total" :value="source.pages_read" :max="source.pages_total" :aria-label="`${source.title}: pages read`" /></span>
        <span class="row-action">{{source.group==='attention'?'Review source':source.group==='processing'?'View progress':'View source'}} <span aria-hidden="true">↗</span></span>
      </button>
      <div class="library-footer"><span role="status">{{(page-1)*12+1}}–{{Math.min(page*12,filtered.length)}} of {{filtered.length}} sources</span><div><button :disabled="page===1" aria-label="Previous source page" @click="page--">Previous</button><button :disabled="page>=pageCount" aria-label="Next source page" @click="page++">Next</button></div></div>
    </template>
  </section>
</template>

<style scoped>
.library{border:1px solid #dce4df;border-radius:14px;background:#fff;overflow:hidden;margin:1.5rem 0}
.listing-limit{margin:0;padding:0 1.5rem 1rem;color:#68776f;font-size:.8rem}
.library-heading{display:flex;align-items:center;justify-content:space-between;padding:1.4rem 1.5rem;gap:1rem}.library-heading h2{margin:0;font-size:1.15rem}.library-heading p{margin:.4rem 0 0;color:#68776f;font-size:.9rem}.total{white-space:nowrap;color:#68776f;font-size:.85rem}
.library-filters{display:flex;gap:1rem;padding:0 1.5rem;overflow:auto;border-bottom:1px solid #e5eae7}.library-filters button{white-space:nowrap;border:0;border-radius:0;background:none;padding:.8rem 0;color:#68776f;border-bottom:2px solid transparent}.library-filters button[aria-pressed=true]{color:#19583e;border-bottom-color:#23754f;font-weight:600}.library-filters span{display:inline-block;margin-left:.25rem;background:#eff3f0;padding:.1rem .4rem;border-radius:5px;font-size:.75rem}
.library-toolbar{display:flex;align-items:end;gap:1rem;padding:1.2rem 1.5rem}.library-toolbar label{font-size:.8rem;font-weight:600;color:#596962}.library-toolbar input,.library-toolbar select{display:block;margin-top:.4rem;font-weight:400;min-height:42px}.library-search{flex:1}.library-search input{width:100%;max-width:440px}
.library-columns,.library-row{display:grid;grid-template-columns:minmax(0,1fr) minmax(160px,.55fr) 115px;gap:1.5rem;align-items:center;padding:1rem 1.5rem}.library-columns{background:#f7f9f7;border-block:1px solid #e5eae7;color:#748078;font-size:.7rem;font-weight:600;text-transform:uppercase;letter-spacing:.08em;padding-block:.7rem}.library-row{width:100%;border:0;border-bottom:1px solid #edf0ee;border-radius:0;text-align:left;background:#fff}.library-row:hover{background:#f7faf8}.document-info{display:flex;gap:.9rem;align-items:center;min-width:0}.document-info>span:last-child{min-width:0}.document-info strong{font-size:.93rem;line-height:1.5;display:block;overflow-wrap:anywhere}.document-info small{display:block;margin-top:.3rem;color:#748078;font-size:.8rem}.document-icon{display:grid;place-items:center;width:38px;height:46px;flex-shrink:0;background:#eff4f0;border:1px solid #dee7e0;border-radius:6px;font-size:1.4rem;color:#52755d}.document-status{display:grid;gap:.5rem;justify-items:start}.status-badge{font-size:.75rem;line-height:1.45;border-radius:6px;padding:.3rem .5rem;background:#edf1ee;color:#657169}.status-badge.ready{background:#e8f4ec;color:#226141}.status-badge.attention{background:#fff2dc;color:#895717}.status-badge.processing{background:#edf2ff;color:#496396}.row-action{font-size:.8rem;font-weight:600;color:#35684d}.row-action span{margin-left:.3rem}progress{width:100%;max-width:150px;height:5px;accent-color:#547b61}.library-empty{text-align:center;padding:3.5rem 1.5rem;color:#68776f}.library-empty h3{color:#263d30;margin-top:1rem}.library-empty p{line-height:1.6}.empty-symbol{font-size:1.7rem;display:inline-grid;place-items:center;border-radius:12px;background:#eff5f0;width:54px;height:54px;color:#52755d}.library-footer{display:flex;align-items:center;justify-content:space-between;padding:1rem 1.5rem;font-size:.8rem;color:#748078;gap:1rem}.library-footer button{font-size:.8rem;margin-left:.4rem}
@media(max-width:700px){.library-heading,.library-toolbar,.library-row,.library-footer{padding-inline:1rem}.library-heading{align-items:start}.library-heading p{max-width:28ch}.library-filters{padding-inline:1rem}.library-toolbar{flex-wrap:wrap}.library-search{flex-basis:100%}.library-search input{max-width:none}.library-columns{display:none}.library-row{grid-template-columns:1fr;gap:.7rem}.document-status{padding-left:53px}.row-action{padding-left:53px}.library-footer{flex-wrap:wrap}.total{display:none}}
</style>
