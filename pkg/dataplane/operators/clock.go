package operators

import "time"

// nowFn is overridable in tests. Operators only read it; the production
// dataplane never replaces it.
var nowFn = time.Now
