-- Migration 034: Batch Persona Pipelines and Pipeline Items
CREATE TABLE IF NOT EXISTS batch_persona_pipelines (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    title VARCHAR(128) NOT NULL,
    status VARCHAR(32) NOT NULL DEFAULT 'queued', -- queued, running, paused, completed, failed, cancelled
    scope_type VARCHAR(32) NOT NULL,              -- group, top_active, custom_qqs, all_persons
    scope_target VARCHAR(128),
    total_targets INT NOT NULL DEFAULT 0,
    completed_targets INT NOT NULL DEFAULT 0,
    failed_targets INT NOT NULL DEFAULT 0,
    concurrency INT NOT NULL DEFAULT 2,
    auto_retry BOOLEAN NOT NULL DEFAULT true,
    max_retries INT NOT NULL DEFAULT 3,
    summary_data JSONB,                          -- 任务画像总结聚合结果
    error_message TEXT,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS batch_pipeline_items (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    pipeline_id UUID NOT NULL REFERENCES batch_persona_pipelines(id) ON DELETE CASCADE,
    person_id UUID NOT NULL REFERENCES persons(id) ON DELETE CASCADE,
    status VARCHAR(32) NOT NULL DEFAULT 'pending', -- pending, processing, completed, failed
    attempts INT NOT NULL DEFAULT 0,
    persona_inference_id UUID REFERENCES inferences(id) ON DELETE SET NULL,
    error_message TEXT,
    started_at TIMESTAMP WITH TIME ZONE,
    finished_at TIMESTAMP WITH TIME ZONE
);

CREATE INDEX IF NOT EXISTS idx_batch_persona_pipelines_status ON batch_persona_pipelines(status);
CREATE INDEX IF NOT EXISTS idx_batch_pipeline_items_pipeline_id ON batch_pipeline_items(pipeline_id);
CREATE INDEX IF NOT EXISTS idx_batch_pipeline_items_person_id ON batch_pipeline_items(person_id);
