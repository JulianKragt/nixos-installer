// Package stages holds the stages of the install pipeline, one file per stage,
// and the helpers they share.
package stages

import "installer/internal/pipeline"

// All is the pipeline: the stages in the order they run.
var All = []pipeline.Stage{
	{ID: "0.0", Name: "Environment preparation", Always: true, Run: environmentPreparation},
}
