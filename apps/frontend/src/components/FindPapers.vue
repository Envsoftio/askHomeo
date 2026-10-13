<script setup lang="ts">
import { ref, onMounted, onUnmounted } from 'vue'
import { api } from '../api'
type Paper = {
  doi: string
  pmid: string
  pmcid: string
  title: string
  authors: string
  year: number
  publication_type: string
  provider: string
  native_id: string
  identity_conflict?: boolean
}
type Reference = Paper & {
  id: string
  source_id: string
  paper_search_id: string
  access_status: string
  rights_status: string
  processing_status: string
  study_country: string
  study_country_review_status: string
  study_country_evidence_block_id: string
  study_country_reason: string
  notice_url: string
  notice_type: string
  notice_checked_at: string
  acquisition_job_id: string
  acquisition_status: string
  acquisition_reason: string
}
type Search = {
  id: string
  state: string
  results: Paper[]
  pages: {
    provider: string
    native_query: string
    total: number
    next: string
    cap: number
    retrieved_at: string
    results: Paper[]
  }[]
  failures: Record<string, string>
}
const emit = defineEmits<{
  (e: 'review', id: string): void
  (e: 'research', id: string, searchId: string): void
}>()
defineProps<{ admin: boolean; readyIds: string[] }>()
const query = ref(''),
  provider = ref('all'),
  busy = ref(false),
  error = ref(''),
  notice = ref(''),
  search = ref<Search | null>(null),
  saved = ref<Reference[]>([])
const locations = ref<
  Record<
    string,
    {
      reason: string
      candidates: { url: string; format: string; version: string; license: string }[]
    }
  >
>({})
type EvidenceBlock = {
  id: string
  kind: string
  heading: string
  original_text: string
  review_status: string
}
const locationReview = ref<{
  workId: string
  country: string
  blockId: string
  reason: string
  filter: string
  blocks: EvidenceBlock[]
} | null>(null)
function evidenceOptions() {
  const review = locationReview.value
  if (!review) return []
  const q = review.filter.toLowerCase()
  return review.blocks
    .filter((b) => (b.heading + ' ' + b.original_text).toLowerCase().includes(q))
    .slice(0, 80)
}
async function openLocation(p: Reference) {
  busy.value = true
  error.value = ''
  try {
    const blocks = p.source_id
      ? await api<EvidenceBlock[]>('/sources/' + p.source_id + '/blocks')
      : []
    locationReview.value = {
      workId: p.id,
      country: p.study_country,
      blockId: p.study_country_evidence_block_id || '',
      reason: p.study_country_reason || '',
      filter: 'methods',
      blocks: blocks.filter(
        (b) => ['accepted', 'corrected'].includes(b.review_status) && b.kind !== 'reference',
      ),
    }
  } catch (e) {
    error.value = e instanceof Error ? e.message : String(e)
  } finally {
    busy.value = false
  }
}
async function saveLocation() {
  const review = locationReview.value
  if (!review) return
  busy.value = true
  error.value = ''
  try {
    await api('/papers/references/' + review.workId + '/study-location', {
      method: 'PUT',
      body: JSON.stringify({
        study_country: review.country,
        evidence_block_id: review.blockId,
        reason: review.reason,
      }),
    })
    await refresh()
    locationReview.value = null
    notice.value = 'Study location reviewed against the selected saved block.'
  } catch (e) {
    error.value = e instanceof Error ? e.message : String(e)
  } finally {
    busy.value = false
  }
}
async function refresh() {
  try {
    saved.value = await api<Reference[]>('/papers/references')
  } catch (e) {
    error.value = e instanceof Error ? e.message : String(e)
  }
}
async function find(next?: { provider: string; next: string }) {
  busy.value = true
  error.value = ''
  try {
    const choice = next?.provider || provider.value
    const continuation = next?.next
      ? choice === 'europepmc'
        ? '&cursor=' + encodeURIComponent(next.next)
        : '&offset=' + encodeURIComponent(next.next)
      : ''
    search.value = await api<Search>(
      '/papers/search?q=' + encodeURIComponent(query.value) + '&provider=' + choice + continuation,
    )
  } catch (e) {
    error.value = e instanceof Error ? e.message : String(e)
  } finally {
    busy.value = false
  }
}
async function save(p: Paper) {
  if (!search.value) return
  busy.value = true
  error.value = ''
  try {
    await api('/papers/references', {
      method: 'POST',
      body: JSON.stringify({
        search_id: search.value.id,
        provider: p.provider,
        native_id: p.native_id,
      }),
    })
    await refresh()
    notice.value = 'Reference saved. Metadata is not answer evidence.'
  } catch (e) {
    error.value = e instanceof Error ? e.message : String(e)
  } finally {
    busy.value = false
  }
}
async function acquire(p: Reference) {
  busy.value = true
  error.value = ''
  try {
    await api('/papers/references/' + p.id + '/acquire', { method: 'POST' })
    await refresh()
    notice.value =
      'Acquisition queued. Progress and failures remain visible here; review is required before publication.'
  } catch (e) {
    error.value = e instanceof Error ? e.message : String(e)
  } finally {
    busy.value = false
  }
}
async function changeJob(p: Reference, action: 'cancel' | 'retry') {
  busy.value = true
  error.value = ''
  try {
    await api('/papers/acquisitions/' + p.acquisition_job_id + '/' + action, { method: 'POST' })
    await refresh()
  } catch (e) {
    error.value = e instanceof Error ? e.message : String(e)
  } finally {
    busy.value = false
  }
}
async function resolve(p: Reference) {
  busy.value = true
  error.value = ''
  try {
    locations.value[p.id] = await api('/papers/references/' + p.id + '/locations')
  } catch (e) {
    error.value = e instanceof Error ? e.message : String(e)
  } finally {
    busy.value = false
  }
}
async function checkNotice(p: Reference) {
  busy.value = true
  error.value = ''
  try {
    await api('/papers/references/' + p.id + '/refresh-notice', { method: 'POST' })
    await refresh()
    notice.value = 'Notice check saved. A recorded notice blocks new evidence use.'
  } catch (e) {
    error.value = e instanceof Error ? e.message : String(e)
  } finally {
    busy.value = false
  }
}
let timer: ReturnType<typeof setInterval> | undefined
onMounted(() => {
  void refresh()
  timer = setInterval(() => {
    void refresh()
  }, 5000)
})
onUnmounted(() => {
  if (timer) clearInterval(timer)
})
</script>
<template>
  <section class="find-papers">
    <p class="eyebrow">SCHOLARLY DISCOVERY</p>
    <h1>Find papers</h1>
    <p>
      Search snapshots show provider coverage and limits. Save a reference to track a publication;
      only reviewed, rights approved and indexed sources can answer questions.
    </p>
    <form @submit.prevent="find()">
      <label
        >Topic
        <input
          v-model="query"
          required
          minlength="3"
          maxlength="250"
          placeholder="condition, intervention or study design" /></label
      ><label
        >Provider
        <select v-model="provider">
          <option value="all">PubMed and Europe PMC</option>
          <option value="pubmed">PubMed</option>
          <option value="europepmc">Europe PMC</option>
          <option value="semantic_scholar">Semantic Scholar (optional)</option>
        </select></label
      ><button :disabled="busy">Search</button>
    </form>
    <p v-if="error" class="error" role="alert">{{ error }}</p>
    <p v-if="notice" role="status">{{ notice }}</p>
    <div v-if="search">
      <p>
        <strong>{{ search.state }}</strong> search · {{ search.results.length }} distinct visible
        publications in this snapshot
      </p>
      <p v-for="page in search.pages" :key="page.provider">
        {{ page.provider }}: {{ page.results.length }} returned; {{ page.total }} provider total;
        checked {{ page.retrieved_at }}.
        {{ page.next ? 'More results exist.' : 'No continuation returned.' }}
        <button v-if="page.next" :disabled="busy" @click="find(page)">Fetch next page</button>
      </p>
      <p v-for="(reason, name) in search.failures" :key="name" class="error">
        {{ name }} failed: {{ reason }}
      </p>
      <article v-for="p in search.results" :key="p.provider + p.native_id">
        <h2>{{ p.title }}</h2>
        <p>
          {{ p.authors }} · {{ p.year || 'year unknown' }} ·
          {{ p.publication_type || 'type unknown' }}
        </p>
        <p>
          {{ p.provider }} · DOI {{ p.doi || 'unknown' }} · PMID {{ p.pmid || 'unknown' }} · PMCID
          {{ p.pmcid || 'unknown' }}
        </p>
        <p>
          Study country: unknown until methods or registry review. Access and rights: unverified.
        </p>
        <button :disabled="busy || p.identity_conflict" @click="save(p)">Save reference</button
        ><span v-if="p.identity_conflict"> Identifier conflict requires review.</span>
      </article>
    </div>
    <h2>Saved references</h2>
    <p v-if="!saved.length">No saved references yet.</p>
    <article v-for="p in saved" :key="p.id">
      <h3>{{ p.title }}</h3>
      <p>
        {{ p.authors }} · {{ p.year || 'year unknown' }} ·
        {{ p.publication_type || 'type unknown' }}
      </p>
      <p>
        DOI {{ p.doi || 'unknown' }} · PMID {{ p.pmid || 'unknown' }} · PMCID
        {{ p.pmcid || 'unknown' }}
      </p>
      <p>
        Study country: {{ p.study_country }} ({{ p.study_country_review_status }})
        <span v-if="p.study_country_evidence_block_id"
          >· saved block {{ p.study_country_evidence_block_id }}</span
        >
        · access {{ p.access_status }} · rights {{ p.rights_status }} · processing
        {{ p.processing_status }} · notice {{ p.notice_type || 'unclassified' }}
        {{ p.notice_url || 'none reported at last check' }} · checked
        {{ p.notice_checked_at || 'never' }}
      </p>
      <p v-if="p.acquisition_job_id">
        Acquisition: {{ p.acquisition_status }} {{ p.acquisition_reason }}
      </p>
      <button v-if="p.doi" :disabled="busy" @click="resolve(p)">Check OA locations</button>
      <button v-if="admin && p.doi" :disabled="busy" @click="checkNotice(p)">Refresh notice</button>
      <button
        v-if="admin && !p.source_id && (p.pmcid || p.doi) && !p.acquisition_job_id && !p.notice_url"
        :disabled="busy"
        @click="acquire(p)"
      >
        Acquire permitted article for review
      </button>
      <button
        v-if="admin && ['queued', 'running'].includes(p.acquisition_status)"
        :disabled="busy"
        @click="changeJob(p, 'cancel')"
      >
        Cancel acquisition
      </button>
      <button
        v-if="admin && ['failed', 'cancelled'].includes(p.acquisition_status) && !p.notice_url"
        :disabled="busy"
        @click="changeJob(p, 'retry')"
      >
        Retry acquisition
      </button>
      <button v-if="p.source_id" @click="emit('review', p.source_id)">Open source</button>
      <button v-if="admin" :disabled="busy" @click="openLocation(p)">Review study location</button>
      <button
        v-if="p.source_id && p.paper_search_id && readyIds.includes(p.source_id) && !p.notice_url"
        @click="emit('research', p.source_id, p.paper_search_id)"
      >
        Use in research</button
      ><span v-else-if="p.source_id">
        Research waits for compatible READY indexing, saved search or notice review.</span
      >
      <div v-if="locations[p.id]">
        <p>
          {{
            locations[p.id].reason ||
            'Candidate locations require independent rights and identity checks.'
          }}
        </p>
        <p v-for="loc in locations[p.id].candidates" :key="loc.url">
          <a :href="loc.url" target="_blank" rel="noopener noreferrer"
            >{{ loc.format }} candidate</a
          >
          · {{ loc.version || 'version unknown' }} · {{ loc.license || 'license unknown' }}
        </p>
      </div>
    </article>
    <section v-if="locationReview" class="location-review">
      <h2>Review study location</h2>
      <p>
        Select a saved methods or registry block. Author affiliation alone does not establish a
        study site.
      </p>
      <label
        >Study country
        <input
          v-model="locationReview.country"
          list="study-country-options"
          placeholder="IN, another ISO code, multinational or unknown" /></label
      ><datalist id="study-country-options">
        <option value="IN"></option>
        <option value="multinational"></option>
        <option value="unknown"></option></datalist
      ><label v-if="locationReview.country !== 'unknown'"
        >Find evidence blocks
        <input v-model="locationReview.filter" placeholder="methods, hospital, site" /></label
      ><label v-if="locationReview.country !== 'unknown'"
        >Saved evidence block
        <select v-model="locationReview.blockId">
          <option value="">Select reviewed block</option>
          <option v-for="b in evidenceOptions()" :key="b.id" :value="b.id">
            {{ b.heading }} · {{ b.original_text.slice(0, 120) }}
          </option>
        </select></label
      ><label>Review reason <textarea v-model="locationReview.reason" rows="3"></textarea></label
      ><button :disabled="busy" @click="saveLocation">Save location review</button
      ><button @click="locationReview = null">Cancel</button>
    </section>
  </section>
</template>
<style scoped>
.find-papers {
  max-width: 76rem;
  margin: 2rem auto;
  padding: 0 1rem;
}
.find-papers form {
  display: flex;
  gap: 0.75rem;
  align-items: end;
  flex-wrap: wrap;
}
.find-papers label {
  display: grid;
  gap: 0.25rem;
}
.find-papers input {
  min-width: 23rem;
}
.find-papers article {
  border: 1px solid #bbb;
  border-radius: 0.5rem;
  padding: 1rem;
  margin: 1rem 0;
}
.find-papers h2,
.find-papers h3 {
  margin: 0.25rem 0;
}
.find-papers p {
  margin: 0.5rem 0;
}
</style>
