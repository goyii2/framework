// Package behaviors provides the Go-shaped counterparts of the behaviors
// Yii2 models attach through their behaviors() method: Timestamp (creation
// and modification times), Blameable (who created or last changed a row),
// Sluggable (URL slugs derived from a source field) and OptimisticLock
// (lost-update detection through a version column).
//
// The landing shape is struct embedding plus GORM lifecycle hooks — Yii2's
// dynamic attach mechanism is deliberately not reproduced. A model embeds
// the behavior structs it wants, plus one behaviors.Hooks to wire them into
// GORM's lifecycle:
//
//	type Article struct {
//	    ar.Model                // instance persistence: Save/Delete/Reload
//	    behaviors.Hooks         // runs the embedded behaviors below
//	    behaviors.Timestamp     // created_at / updated_at, time.Time
//	    behaviors.Sluggable     // slug derived from a source field
//	}
//
// Hooks exists because GORM invokes lifecycle hooks by method name, and two
// embedded structs both defining BeforeCreate would silently cancel each
// other out (their promoted methods collide in Go). Hooks is the single
// promoted hook implementation; it walks the model's embedded fields on
// each event and hands every behavior its turn. A model that defines its
// own BeforeCreate/BeforeUpdate loses the promoted Hooks methods (the
// shallower definition wins) and must delegate through DispatchCreate,
// DispatchUpdate and DispatchUpdated to keep its behaviors running.
//
// All behaviors fill fields they declare themselves, so a model can embed
// ar.Model, Hooks and any combination of behaviors side by side; none of
// them contributes methods that collide with another.
package behaviors
