#!/usr/bin/env Rscript

# This script is intended to trigger the actual report generation using a
# R markdown file as a template.

# Install missing packages.
list.of.packages <- c("rmarkdown", "tidyverse", "lubridate", "slider")
new.packages <- list.of.packages[!(list.of.packages %in% installed.packages()[,"Package"])]
if(length(new.packages)) install.packages(new.packages)

library("rmarkdown")

# test that there are exactly 5 arguments
#  - the report template to render
#  - the data source file
#  - the output directory
#  - the output file name
#  - the scenario name
args <- commandArgs(trailingOnly=TRUE)
if (length(args) != 5) {
  stop("Script requires exactly five parameters: <template> <data> <outputdir> <outputfile> <scenario>", call.=FALSE)
}

template <- args[1]
data <- args[2]
outputdir <- args[3]
outputfile <- args[4]
scenario <- args[5]

rmarkdown::render(
    template,
    params = list(datafile = data, scenario = scenario),
    output_dir = outputdir,
    output_file = outputfile,
    intermediates_dir = outputdir,
)