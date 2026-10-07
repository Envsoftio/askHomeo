ALTER TABLE answer_searches DROP CONSTRAINT IF EXISTS answer_searches_source_scope_check;
ALTER TABLE answer_searches ADD CONSTRAINT answer_searches_source_scope_check CHECK (length(source_scope) BETWEEN 1 AND 300);

-- Correct the exact local PDF whose generic first-page header was mistaken for
-- its title and whose text fragment "the ROBIS" was mistaken for its author.
UPDATE sources SET
 title='Efficacy of homoeopathic treatment: systematic review of meta-analyses of randomised placebo-controlled homoeopathy trials for any indication',
 author='H. J. Hamre, A. Glockmann, K. von Ammon, D. S. Riley, H. Kiene',
 edition='',
 publication_info='Systematic Reviews (2023) 12:191',
 repository='Systematic Reviews / BMC',
 source_url='https://doi.org/10.1186/s13643-023-02313-2',
 updated_at=now()
WHERE pdf_sha256='de3422a39bd42a9374d27555ea5f89a5dc9ddada732f0356a5d46c89d4a8d848'
 AND title='et al. Systematic Reviews (2023) 12:191'
 AND author='the ROBIS';
