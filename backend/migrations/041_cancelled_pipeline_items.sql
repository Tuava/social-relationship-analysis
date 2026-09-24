UPDATE batch_pipeline_items AS item
SET status = 'cancelled',
    finished_at = COALESCE(item.finished_at, pipeline.updated_at),
    error_message = COALESCE(item.error_message, '任务已取消')
FROM batch_persona_pipelines AS pipeline
WHERE item.pipeline_id = pipeline.id
  AND pipeline.status = 'cancelled'
  AND item.status IN ('pending', 'processing');
