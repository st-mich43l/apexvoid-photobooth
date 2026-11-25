# Frontend source layout

- `app/` contains application startup, global state, and top-level composition.
- `features/` contains code grouped by user-facing capability. Each feature owns its pages, components, styles, and focused UI tests.
- `shared/` contains reusable API clients, domain types, localization, and UI primitives used across features.

Feature code may depend on `shared/` and `app/`. Keep feature-specific components inside their owning feature instead of adding new files at the source root.
