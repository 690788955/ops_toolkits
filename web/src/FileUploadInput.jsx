import React, {useState} from 'react'
import {postPlatformFile} from './api.js'
import {readableAPIError} from './utils.js'

export default function FileUploadInput({value, onChange, disabled = false}) {
  const [uploading, setUploading] = useState(false)
  const [message, setMessage] = useState('')

  async function upload(event) {
    const file = event.target.files?.[0]
    if (!file) return
    setUploading(true)
    setMessage('上传中...')
    try {
      const body = await postPlatformFile(file)
      const data = body.data || body
      onChange(data.path || '')
      setMessage(data.path ? `已上传：${data.filename || file.name}` : '上传完成')
    } catch (err) {
      setMessage(readableAPIError(err, '上传失败。'))
    } finally {
      setUploading(false)
      event.target.value = ''
    }
  }

  return (
    <div className="fileUploadInput">
      <input value={value || ''} placeholder="上传后自动填入平台文件路径" onChange={event => onChange(event.target.value)} disabled={disabled || uploading} />
      <label className="secondary fileUploadButton">
        <input type="file" onChange={upload} disabled={disabled || uploading} />
        <span>{uploading ? '上传中' : '选择文件'}</span>
      </label>
      {message && <small>{message}</small>}
    </div>
  )
}
