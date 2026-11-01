-- +goose Up
-- campaign_content gets a tenth kind, 'creature': an SRD creature with a few adjustments (hit
-- points, armor class, a damage bonus, extra traits). Its key is "creature:<slug>@mesa" and its
-- data is the protojson of TableCreature in rules/v1/table_content.proto.
ALTER TABLE campaign_content DROP CONSTRAINT IF EXISTS campaign_content_kind_valid;
ALTER TABLE campaign_content ADD CONSTRAINT campaign_content_kind_valid
    CHECK (kind IN ('class', 'subclass', 'race', 'subrace', 'background', 'spell', 'feat', 'option', 'classfeature', 'creature'));

-- +goose Down
ALTER TABLE campaign_content DROP CONSTRAINT IF EXISTS campaign_content_kind_valid;
ALTER TABLE campaign_content ADD CONSTRAINT campaign_content_kind_valid
    CHECK (kind IN ('class', 'subclass', 'race', 'subrace', 'background', 'spell', 'feat', 'option', 'classfeature'));
