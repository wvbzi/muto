# Muto - YouTube to MP3 Discord Bot

Takes a YouTube share link via Discord slash command and serves
back an MP3.

## Versions 

### Current (AWS Deployment)
- Handles videos up to 30 minutes
- ECS/EC2 deployment on t3.micro instance
- S3 storage and CloudFront CDN distribution
- Signed URL generation with custom domain for MP3 downloads
- Concurrent download management (max 3 simultaneous)
- Proxy support to handle YouTube's server IP restrictions
- Stays within AWS Free Tier :)

### [Lite](https://github.com/wvbzi/muto/tree/muto-lite)
- For local hosting
- 8MB file limit
- MP3 downloads served directly through Discord

## Setup

Detailed setup instructions soon. Requires:
- AWS Account (ECR, ECS, S3, CloudFront)
- Discord Bot Token
- Docker
- Proxies (Residential recommended)