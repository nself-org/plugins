# File Processing Plugin - Quick Start

Get started with file processing in under 5 minutes.

## Prerequisites

```bash
# Node.js 18+
node --version

# PostgreSQL
psql --version

# Redis
redis-cli ping

# Optional: ffmpeg (for video thumbnails)
ffmpeg -version

# Optional: ClamAV (for virus scanning)
clamd --version
```

## Installation

### 1. Install the Plugin

```bash
cd ~/Sites/nself-plugins/plugins/file-processing
./install.sh
```

### 2. Configure Environment

Copy `.env.example` to `ts/.env` and configure:

```bash
cd ts
cp ../.env.example .env
```

Edit `.env` with your settings:

```bash
# Minimum required configuration
FILE_STORAGE_PROVIDER=minio
FILE_STORAGE_BUCKET=files
FILE_STORAGE_ENDPOINT=http://localhost:9000
FILE_STORAGE_ACCESS_KEY=minioadmin
FILE_STORAGE_SECRET_KEY=minioadmin
DATABASE_URL=postgresql://user:pass@localhost:5432/nself
REDIS_URL=redis://localhost:6379
```

### 3. Build TypeScript

```bash
npm install
npm run build
```

## Usage

### Start Services

Terminal 1 - HTTP Server:
```bash
nself plugin file-processing server
# Or: cd ts && npm run dev
```

Terminal 2 - Background Worker:
```bash
nself plugin file-processing worker
# Or: cd ts && npm run worker
```

### Process a File

```bash
# Via CLI
nself plugin file-processing process file_123 uploads/photo.jpg

# Via HTTP API
curl -X POST http://localhost:3104/api/jobs \
  -H "Content-Type: application/json" \
  -d '{
    "fileId": "file_123",
    "filePath": "uploads/photo.jpg",
    "fileName": "photo.jpg",
    "fileSize": 1024000,
    "mimeType": "image/jpeg",
    "operations": ["thumbnail", "optimize", "metadata"]
  }'
```

### Check Status

```bash
# View statistics
nself plugin file-processing stats

# Check specific job
curl http://localhost:3104/api/jobs/{job-id}
```

## Integration Example

### Node.js/TypeScript

```typescript
// Upload file and process
async function uploadAndProcess(file: File) {
  // 1. Upload to storage (your upload logic)
  const uploadResult = await uploadToStorage(file);

  // 2. Request processing
  const response = await fetch('http://localhost:3104/api/jobs', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({
      fileId: uploadResult.id,
      filePath: uploadResult.path,
      fileName: file.name,
      fileSize: file.size,
      mimeType: file.type,
      operations: ['thumbnail', 'optimize', 'metadata'],
      webhookUrl: 'https://myapp.com/webhooks/file-processed',
      webhookSecret: 'your_secret',
    }),
  });

  const { jobId } = await response.json();
  console.log('Processing job created:', jobId);

  // 3. Poll for results or wait for webhook
  const result = await pollJobStatus(jobId);
  console.log('Thumbnails:', result.thumbnails);
}
```

### React Component

```tsx
import { useState } from 'react';

function FileUploader() {
  const [processing, setProcessing] = useState(false);
  const [thumbnails, setThumbnails] = useState([]);

  const handleUpload = async (file: File) => {
    setProcessing(true);

    // Upload file
    const formData = new FormData();
    formData.append('file', file);
    const upload = await fetch('/api/upload', {
      method: 'POST',
      body: formData,
    });
    const { fileId, filePath } = await upload.json();

    // Process file
    const process = await fetch('http://localhost:3104/api/jobs', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({
        fileId,
        filePath,
        fileName: file.name,
        fileSize: file.size,
        mimeType: file.type,
        operations: ['thumbnail'],
      }),
    });

    const { jobId } = await process.json();

    // Poll for completion
    const checkStatus = setInterval(async () => {
      const status = await fetch(`http://localhost:3104/api/jobs/${jobId}`);
      const data = await status.json();

      if (data.job.status === 'completed') {
        clearInterval(checkStatus);
        setThumbnails(data.thumbnails);
        setProcessing(false);
      }
    }, 1000);
  };

  return (
    <div>
      <input type="file" onChange={(e) => e.target.files && handleUpload(e.target.files[0])} />
      {processing && <p>Processing...</p>}
      {thumbnails.map((thumb) => (
        <img key={thumb.id} src={thumb.url} alt="Thumbnail" />
      ))}
    </div>
  );
}
```

## Storage Provider Examples

### AWS S3

```bash
FILE_STORAGE_PROVIDER=s3
FILE_STORAGE_BUCKET=my-bucket
FILE_STORAGE_REGION=us-east-1
FILE_STORAGE_ACCESS_KEY=AKIAIOSFODNN7EXAMPLE
FILE_STORAGE_SECRET_KEY=wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY
```

### Cloudflare R2

```bash
FILE_STORAGE_PROVIDER=r2
FILE_STORAGE_BUCKET=my-bucket
FILE_STORAGE_ENDPOINT=https://[account-id].r2.cloudflarestorage.com
FILE_STORAGE_ACCESS_KEY=your_r2_access_key
FILE_STORAGE_SECRET_KEY=your_r2_secret_key
FILE_STORAGE_REGION=auto
```

### Google Cloud Storage

```bash
FILE_STORAGE_PROVIDER=gcs
FILE_STORAGE_BUCKET=my-bucket
GOOGLE_APPLICATION_CREDENTIALS=/path/to/service-account-key.json
```

## Troubleshooting

### Server won't start
```bash
# Check if port is in use
lsof -i :3104

# Check configuration
nself plugin file-processing init
```

### Worker not processing
```bash
# Check Redis connection
redis-cli ping

# Check queue
redis-cli KEYS "bull:file-processing:*"
```

### Thumbnails not generating
```bash
# Check Sharp installation
cd ts && npm rebuild sharp

# Check ffmpeg (for videos)
which ffmpeg
```

## Next Steps

1. **Set up webhooks** - Get notified when processing completes
2. **Enable virus scanning** - Install ClamAV and set `FILE_ENABLE_VIRUS_SCAN=true`
3. **Customize thumbnails** - Adjust `FILE_THUMBNAIL_SIZES` to your needs
4. **Scale workers** - Run multiple worker instances for higher throughput
5. **Monitor performance** - Use `nself plugin file-processing stats` regularly

## Support

- Full Documentation: [README.md](./README.md)
- GitHub Issues: https://github.com/acamarata/nself-plugins/issues
- nself CLI Docs: https://github.com/acamarata/nself
