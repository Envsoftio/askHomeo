CREATE TABLE answer_source_filters (
    answer_id UUID NOT NULL REFERENCES answers(id) ON DELETE CASCADE,
    source_id UUID NOT NULL REFERENCES sources(id),
    ordinal INT NOT NULL CHECK (ordinal > 0),
    PRIMARY KEY (answer_id, source_id),
    UNIQUE (answer_id, ordinal)
);
