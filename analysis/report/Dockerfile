# Dockerfile for the Norma R report renderer.
# Build with: make build-r-renderer-image
#
# rocker/tidyverse already includes R, rmarkdown, knitr, pandoc, and the full
# tidyverse (including lubridate). Only `slider` needs to be added on top.
FROM rocker/tidyverse:4.4

RUN Rscript -e "install.packages('slider', repos='https://cloud.r-project.org')"