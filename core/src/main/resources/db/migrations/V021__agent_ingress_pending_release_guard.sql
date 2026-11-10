-- Historical pending rows may have reached Kafka. Treat them as publication-uncertain.
ALTER TABLE platform.sensor_agent_ingress_watermark
    ADD COLUMN pending_reservation_id UUID,
    ADD COLUMN pending_publication_attempted BOOLEAN NOT NULL DEFAULT FALSE;

UPDATE platform.sensor_agent_ingress_watermark
   SET pending_reservation_id = gen_random_uuid(),
       pending_publication_attempted = TRUE
 WHERE pending_first_sequence IS NOT NULL;

ALTER TABLE platform.sensor_agent_ingress_watermark
    ADD CONSTRAINT chk_agent_ingress_pending_release_guard CHECK (
        (pending_first_sequence IS NULL AND pending_reservation_id IS NULL
            AND pending_publication_attempted = FALSE)
        OR (pending_first_sequence IS NOT NULL AND pending_reservation_id IS NOT NULL)
    );
