// @vitest-environment jsdom

import React, {act, useState} from 'react'
import {createRoot} from 'react-dom/client'
import {expect, it} from 'vitest'
import {NodeConfigEditor} from './inspectors.jsx'
import {serializeApprovalConfig, serializeNotifyConfig, serializeScriptConfig} from './model.js'

it('Shell 环境变量可以添加空行、输入大小写名称并删除', async () => {
  let latest
  function Harness() {
    const [node, setNode] = useState({id: 'shell', type: 'controlNode', data: {controlType: 'shell_script', script: {script: 'true'}}})
    latest = node.data.script
    return <NodeConfigEditor node={node} tools={[]} nodes={[]} hosts={[]} sources={[]} onScriptChange={script => setNode(current => ({...current, data: {...current.data, script}}))} />
  }
  const host = document.createElement('div')
  document.body.appendChild(host)
  const root = createRoot(host)
  try {
    await act(async () => { root.render(<Harness />) })
    const add = Array.from(host.querySelectorAll('button')).find(button => button.textContent === '添加变量')
    await act(async () => { add.click() })
    const row = host.querySelector('.httpHeaderRow')
    expect(row).toBeTruthy()
    const setter = Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, 'value').set
    await act(async () => {
      setter.call(row.querySelector('input'), 'lowerCase')
      row.querySelector('input').dispatchEvent(new Event('input', {bubbles: true}))
    })
    await act(async () => {
      const valueInput = host.querySelector('.httpHeaderRow input:nth-child(2)')
      setter.call(valueInput, 'prod')
      valueInput.dispatchEvent(new Event('input', {bubbles: true}))
    })
    expect(serializeScriptConfig(latest).env).toEqual({lowerCase: 'prod'})
    await act(async () => { host.querySelector('.httpHeaderRow button').click() })
    expect(host.querySelector('.httpHeaderRow')).toBeNull()
    expect(serializeScriptConfig(latest).env).toBeUndefined()
  } finally {
    await act(async () => { root.unmount() })
    host.remove()
  }
})

it('人工审批节点可以填提示与超时，并说明这是停机点', async () => {
  let latest
  function Harness() {
    const [node, setNode] = useState({id: 'gate', type: 'controlNode', data: {controlType: 'approval', approval: {message: '', timeout: ''}}})
    latest = node.data.approval
    return <NodeConfigEditor node={node} tools={[]} nodes={[]} hosts={[]} sources={[]} onApprovalChange={approval => setNode(current => ({...current, data: {...current.data, approval}}))} />
  }
  const host = document.createElement('div')
  document.body.appendChild(host)
  const root = createRoot(host)
  const setValue = (element, value) => {
    const prototype = element.tagName === 'TEXTAREA' ? HTMLTextAreaElement.prototype : HTMLInputElement.prototype
    Object.getOwnPropertyDescriptor(prototype, 'value').set.call(element, value)
    element.dispatchEvent(new Event('input', {bubbles: true}))
  }
  try {
    await act(async () => { root.render(<Harness />) })

    const message = host.querySelector('textarea')
    expect(message.placeholder).toContain('{{ .参数 }}')
    const timeout = Array.from(host.querySelectorAll('input')).find(input => input.placeholder === '可留空=一直等待，例如 30m、2h')
    expect(timeout).toBeTruthy()
    expect(host.textContent).toContain('这是停机点')
    expect(host.textContent).toContain('确认与驳回的入口在运行日志面板的审批条上')
    // 空配置不写任何字段，避免把空字符串当成 0 时长写进 YAML。
    expect(serializeApprovalConfig(latest)).toEqual({})

    const messageSetter = Object.getOwnPropertyDescriptor(HTMLTextAreaElement.prototype, 'value').set
    await act(async () => {
      messageSetter.call(message, '确认配置已改好后继续')
      message.dispatchEvent(new Event('input', {bubbles: true}))
    })
    await act(async () => { setValue(timeout, '1h30m') })

    expect(serializeApprovalConfig(latest)).toEqual({message: '确认配置已改好后继续', timeout: '1h30m'})
  } finally {
    await act(async () => { root.unmount() })
    host.remove()
  }
})

it('通知节点可在 Webhook 与邮件通道间切换并按通道序列化', async () => {
  let latest
  function Harness() {
    const [node, setNode] = useState({id: 'alert', type: 'controlNode', data: {controlType: 'notify', notify: {channel: 'webhook', title: '部署完成'}}})
    latest = node.data.notify
    return <NodeConfigEditor node={node} tools={[]} nodes={[]} hosts={[]} sources={[]} onNotifyChange={notify => setNode(current => ({...current, data: {...current.data, notify}}))} />
  }
  const host = document.createElement('div')
  document.body.appendChild(host)
  const root = createRoot(host)
  const setValue = (element, value) => {
    const prototype = element.tagName === 'SELECT' ? HTMLSelectElement.prototype : element.tagName === 'TEXTAREA' ? HTMLTextAreaElement.prototype : HTMLInputElement.prototype
    Object.getOwnPropertyDescriptor(prototype, 'value').set.call(element, value)
    element.dispatchEvent(new Event(element.tagName === 'SELECT' ? 'change' : 'input', {bubbles: true}))
  }
  try {
    await act(async () => { root.render(<Harness />) })
    const channel = host.querySelector('select')
    await act(async () => { setValue(channel, 'webhook') })
    const webhookInput = Array.from(host.querySelectorAll('input')).find(input => input.placeholder === 'https://hooks.example.com/ops')
    await act(async () => { setValue(webhookInput, 'https://hooks.example.com/ops') })
    expect(serializeNotifyConfig(latest)).toEqual({channel: 'webhook', title: '部署完成', webhook: 'https://hooks.example.com/ops'})

    await act(async () => { setValue(host.querySelector('select'), 'email') })
    expect(host.textContent).toContain('SMTP 服务器')
    const smtpHost = Array.from(host.querySelectorAll('input')).find(input => input.placeholder === 'smtp.example.com')
    const recipients = Array.from(host.querySelectorAll('input')).find(input => input.placeholder.startsWith('多人用逗号分隔'))
    await act(async () => {
      setValue(smtpHost, 'smtp.example.com')
      setValue(recipients, 'ops@example.com, dev@example.com')
    })
    const serialized = serializeNotifyConfig(latest)
    expect(serialized.channel).toBe('email')
    expect(serialized.to).toEqual(['ops@example.com', 'dev@example.com'])
    expect(serialized.webhook).toBeUndefined()
  } finally {
    await act(async () => { root.unmount() })
    host.remove()
  }
})
