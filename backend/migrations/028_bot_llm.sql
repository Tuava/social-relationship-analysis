-- Add LLM configurations to bot_instances for Natural Language Understanding.
ALTER TABLE bot_instances
    ADD COLUMN IF NOT EXISTS llm_provider text NOT NULL DEFAULT 'openai',
    ADD COLUMN IF NOT EXISTS llm_api_base text NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS llm_api_key text NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS llm_model text NOT NULL DEFAULT 'gpt-4o-mini',
    ADD COLUMN IF NOT EXISTS llm_temperature double precision NOT NULL DEFAULT 0.7,
    ADD COLUMN IF NOT EXISTS llm_max_tokens integer NOT NULL DEFAULT 500;
