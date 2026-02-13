package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
	"github.com/google/uuid"
	"github.com/technobecet/kaizoku-go/internal/types"
)

// ProviderStorage holds the schema definition for the ProviderStorage entity.
type ProviderStorage struct {
	ent.Schema
}

func (ProviderStorage) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}).Default(uuid.New).Immutable(),
		field.String("apk_name"),
		field.String("pkg_name"),
		field.String("name").Default(""),
		field.String("lang").Default(""),
		field.Int64("version_code").Default(0),
		field.Bool("is_storage").Default(true),
		field.Bool("is_disabled").Default(false),
		field.JSON("mappings", []types.ProviderMapping{}).Optional(),
	}
}

func (ProviderStorage) Edges() []ent.Edge {
	return nil
}

func (ProviderStorage) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("name", "lang"),
	}
}
