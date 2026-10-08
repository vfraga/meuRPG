-- +goose Up
-- Showing a fog map's image looks for the copy already made of it.
CREATE INDEX IF NOT EXISTS gallery_images_copy_of_image_id_idx
    ON gallery_images (copy_of_image_id)
    WHERE copy_of_image_id IS NOT NULL;

-- +goose Down
DROP INDEX IF EXISTS gallery_images_copy_of_image_id_idx;
