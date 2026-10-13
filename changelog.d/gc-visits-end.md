The pm service no longer dies with "panic: close of closed channel" when a garbage collection follows another one
closely.

### Fixed

- A garbage collection of the work store now returns only once its visits to the other open sessions have ended.
  Dolt's ended a visit after the collection returned, so a collection that began right after another could visit a
  session whose last visit had not ended, and the pm service died ("panic: close of closed channel", inside Dolt).
  The service collects once a day, so this hit back-to-back collections, as pm's own tests run them.
