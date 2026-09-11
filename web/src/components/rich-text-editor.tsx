/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/
import type {
  IDomEditor,
  IEditorConfig,
  IToolbarConfig,
} from '@wangeditor/editor'
import { Editor, Toolbar } from '@wangeditor/editor-for-react'

import '@wangeditor/editor/dist/css/style.css'
import { useEffect, useRef } from 'react'

interface RichTextEditorProps {
  value: string
  onChange: (value: string) => void
  placeholder?: string
}

export function RichTextEditor({
  value,
  onChange,
  placeholder,
}: RichTextEditorProps) {
  const editorRef = useRef<IDomEditor | null>(null)
  const toolbarConfig: Partial<IToolbarConfig> = {
    excludeKeys: ['group-video'],
  }
  const editorConfig: Partial<IEditorConfig> = {
    placeholder,
    MENU_CONF: {
      uploadImage: {
        // Announcement images are embedded by URL; server upload endpoints are not assumed.
        allowedFileTypes: [],
      },
    },
  }

  useEffect(() => {
    return () => {
      editorRef.current?.destroy()
      editorRef.current = null
    }
  }, [])

  return (
    <div className='bg-background overflow-hidden rounded-md border'>
      <Toolbar
        editor={editorRef.current}
        defaultConfig={toolbarConfig}
        mode='default'
      />
      <Editor
        value={value}
        defaultConfig={editorConfig}
        mode='default'
        onCreated={(editor) => {
          editorRef.current = editor
        }}
        onChange={(editor) => onChange(editor.getHtml())}
        style={{ height: '280px', overflowY: 'auto' }}
      />
    </div>
  )
}
