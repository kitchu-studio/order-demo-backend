-- File name of the uploaded photo (inside UPLOAD_DIR). Empty = no photo yet.
ALTER TABLE products ADD COLUMN image_path TEXT NOT NULL DEFAULT '';
