-- +goose Up
ALTER TABLE local_media_libraries
    ADD COLUMN root_paths JSONB NOT NULL DEFAULT '[]'::jsonb;
UPDATE local_media_libraries SET root_paths = jsonb_build_array(root_path);

-- +goose Down
ALTER TABLE local_media_libraries DROP COLUMN root_paths;
