#!/usr/bin/env Rscript

# This script is intended to trigger the actual report generation using a
# R markdown file as a template.

# Install missing packages.
list.of.packages <- c("rmarkdown", "tidyverse", "lubridate", "slider")
new.packages <- list.of.packages[!(list.of.packages %in% installed.packages()[,"Package"])]
if(length(new.packages)) install.packages(new.packages)

library("rmarkdown")

# test that there are exactly 7 arguments
#  - the report template to render
#  - the data source file
#  - the output directory
#  - the output file name
#  - the scenario name
#  - the scenario description
#  - the scenario file path
args <- commandArgs(trailingOnly=TRUE)
if (length(args) != 7) {
  stop("Script requires exactly seven parameters: <template> <data> <outputdir> <outputfile> <scenario> <description> <scenario_file>", call.=FALSE)
}

template <- args[1]
data <- args[2]
outputdir <- args[3]
outputfile <- args[4]
scenario <- args[5]
description <- args[6]
scenario_file <- args[7]

rmarkdown::render(
    template,
  params = list(
    datafile = data,
    scenario = scenario,
    description = description,
    scenario_file = scenario_file
  ),
    output_dir = outputdir,
    output_file = outputfile,
    intermediates_dir = outputdir,
)