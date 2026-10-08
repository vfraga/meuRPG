-- +goose Up
-- copy_of_image_id marks the gallery image that is the copy of a fog map's image made
-- to show it, or to be an NPC's portrait (MR-036, RN-10): showing the same image again
-- finds the copy instead of making another one. Deleting the source keeps the copy and
-- cuts the link (SET NULL): the copy is a complete image on its own.
--
-- One statement with several parts, so re-running it is safe (see 00033).
ALTER TABLE gallery_images
    ADD COLUMN IF NOT EXISTS copy_of_image_id UUID NULL,
    DROP CONSTRAINT IF EXISTS gallery_images_copy_of_image_id_fkey,
    ADD CONSTRAINT gallery_images_copy_of_image_id_fkey
        FOREIGN KEY (copy_of_image_id) REFERENCES gallery_images (id) ON DELETE SET NULL;

-- +goose Down
ALTER TABLE gallery_images
    DROP CONSTRAINT IF EXISTS gallery_images_copy_of_image_id_fkey,
    DROP COLUMN IF EXISTS copy_of_image_id;
