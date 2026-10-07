ALTER TABLE activity_events ADD COLUMN source_job_id uuid REFERENCES jobs(id);
DO $$ DECLARE old_constraint text; BEGIN
 FOR old_constraint IN SELECT conname FROM pg_constraint WHERE conrelid='activity_events'::regclass AND contype='c' LOOP
  EXECUTE format('ALTER TABLE activity_events DROP CONSTRAINT %I',old_constraint);
 END LOOP;
END $$;
ALTER TABLE activity_events ADD CONSTRAINT activity_events_target_check CHECK (num_nonnulls(answer_job_id,source_job_id)=1);

CREATE OR REPLACE FUNCTION record_source_job_activity() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE event_kind text;
DECLARE event_message text;
BEGIN
 IF TG_OP='UPDATE' AND NEW.status=OLD.status AND NEW.current_step=OLD.current_step AND coalesce(NEW.error,'')=coalesce(OLD.error,'') THEN
  RETURN NEW;
 END IF;
 event_kind := CASE NEW.status WHEN 'queued' THEN 'waiting' WHEN 'running' THEN 'working' WHEN 'done' THEN 'finished' ELSE 'failed' END;
 event_message := NEW.current_step;
 IF NEW.status='failed' AND nullif(NEW.error,'') IS NOT NULL THEN event_message := event_message || ': ' || NEW.error; END IF;
 INSERT INTO activity_events(id,source_job_id,kind,message) VALUES(gen_random_uuid(),NEW.id,event_kind,event_message);
 RETURN NEW;
END $$;
CREATE TRIGGER jobs_activity_transition AFTER INSERT OR UPDATE OF status,current_step,error ON jobs
 FOR EACH ROW EXECUTE FUNCTION record_source_job_activity();
