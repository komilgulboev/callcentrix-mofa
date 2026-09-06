import React from 'react'
import TasksBoard from 'src/views/dashboard/TasksBoard'

// Dedicated "Задачи" page for the nav menu — TasksBoard already carries its
// own title/search/Kanban/detail-modal, so this is just the standalone home
// for it (the Dashboard also embeds the same component as an at-a-glance
// widget; both share one implementation, so search/detail/comments work in
// both places for free).
export default function Tasks() {
  return <TasksBoard />
}
