package main

import (
	"mime"
	"strconv"
	"strings"
	"unicode/utf8"
)

// The subject is the head of the message: which project, and what happened. It is read before
// anything is opened, often in a notification strip that shows the first thirty or forty
// characters, so the project comes first and the rest follows in the order a reader asks for it.
//
// Titles stay out of it. Everything else in a subject is vocabulary amenbo owns — a project name,
// one of eleven events, a reference — and is a length that can be reckoned with; a title is
// whatever the user typed, so putting one here would mean cutting it, and a cut title says
// something other than what it said. It is the first thing in the body regardless, so nothing is
// lost by leaving it out.

// subjectLimit is how many characters a subject may run to. Characters, not bytes: a subject with
// anything non-ASCII in it is carried as an encoded word, which is around four times its length
// in bytes, and a byte limit would leave a Japanese subject a quarter the length of an English
// one for no reason a reader would recognise.
const subjectLimit = 60

// fallbackProject is what stands in the brackets when the project's name could not be read back.
// A subject is never empty, and the name of the thing that sent the message is closer to the
// truth than nothing at all.
const fallbackProject = "amenbo"

// ellipsis marks a project name that was cut to fit.
const ellipsis = "…"

// subjectWording is one language's subject vocabulary: a short phrase for each event, and the way
// it counts when a message carries more than one. It is shorter than the wording a line is
// written from — a subject says what happened, and the line says who did it to what.
type subjectWording struct {
	events map[string]string
	// many says how many, with {n} standing for the count.
	many string
	// one is what many says for a count of one, in the languages that say it differently — a
	// message carrying a single event does not always say it in full, since the run that sends
	// it is not always the run the event arrived on. Left empty by every language a numeral
	// changes nothing in, which is most of them outside Europe.
	one string
}

// subjectWords is keyed by the same language codes the sentences are, and a test holds the two
// lists to each other: a language added to one and not the other would write its lines in the
// user's language and its subject in English.
var subjectWords = map[string]subjectWording{
	"en": {
		many: "{n} updates",
		one:  "{n} update",
		events: map[string]string{
			eventTaskCreated: "Task created", eventTaskStatusChanged: "Task moved to {status}",
			eventTaskDone: "Task finished", eventTaskRejected: "Task decided against",
			eventTaskAssigned: "Task reassigned", eventTaskMoved: "Task moved",
			eventTaskDeleted: "Task deleted", eventDecisionAccepted: "Decision accepted",
			eventDecisionRejected: "Decision rejected", eventCommentAdded: "Comment added",
			eventCommentRemoved: "Comment removed",
		},
	},
	"de": {
		many: "{n} Änderungen",
		one:  "{n} Änderung",
		events: map[string]string{
			eventTaskCreated: "Aufgabe erstellt", eventTaskStatusChanged: "Aufgabe auf {status}",
			eventTaskDone: "Aufgabe abgeschlossen", eventTaskRejected: "Aufgabe verworfen",
			eventTaskAssigned: "Aufgabe neu zugewiesen", eventTaskMoved: "Aufgabe verschoben",
			eventTaskDeleted: "Aufgabe gelöscht", eventDecisionAccepted: "Entscheidung angenommen",
			eventDecisionRejected: "Entscheidung abgelehnt", eventCommentAdded: "Kommentar hinzugefügt",
			eventCommentRemoved: "Kommentar zurückgenommen",
		},
	},
	"es": {
		many: "{n} novedades",
		one:  "{n} novedad",
		events: map[string]string{
			eventTaskCreated: "Tarea creada", eventTaskStatusChanged: "Tarea a {status}",
			eventTaskDone: "Tarea terminada", eventTaskRejected: "Tarea descartada",
			eventTaskAssigned: "Tarea reasignada", eventTaskMoved: "Tarea movida",
			eventTaskDeleted: "Tarea eliminada", eventDecisionAccepted: "Decisión aceptada",
			eventDecisionRejected: "Decisión rechazada", eventCommentAdded: "Comentario añadido",
			eventCommentRemoved: "Comentario retirado",
		},
	},
	"fr": {
		many: "{n} mises à jour",
		one:  "{n} mise à jour",
		events: map[string]string{
			eventTaskCreated: "Tâche créée", eventTaskStatusChanged: "Tâche à {status}",
			eventTaskDone: "Tâche terminée", eventTaskRejected: "Tâche écartée",
			eventTaskAssigned: "Tâche réattribuée", eventTaskMoved: "Tâche déplacée",
			eventTaskDeleted: "Tâche supprimée", eventDecisionAccepted: "Décision acceptée",
			eventDecisionRejected: "Décision rejetée", eventCommentAdded: "Commentaire ajouté",
			eventCommentRemoved: "Commentaire retiré",
		},
	},
	"hi": {
		many: "{n} अपडेट",
		events: map[string]string{
			eventTaskCreated: "कार्य बनाया", eventTaskStatusChanged: "कार्य {status}",
			eventTaskDone: "कार्य पूरा", eventTaskRejected: "कार्य छोड़ा",
			eventTaskAssigned: "कार्य सौंपा", eventTaskMoved: "कार्य स्थानांतरित",
			eventTaskDeleted: "कार्य मिटाया", eventDecisionAccepted: "निर्णय स्वीकृत",
			eventDecisionRejected: "निर्णय अस्वीकृत", eventCommentAdded: "टिप्पणी जोड़ी",
			eventCommentRemoved: "टिप्पणी हटाई",
		},
	},
	"id": {
		many: "{n} pembaruan",
		events: map[string]string{
			eventTaskCreated: "Tugas dibuat", eventTaskStatusChanged: "Tugas jadi {status}",
			eventTaskDone: "Tugas selesai", eventTaskRejected: "Tugas dibatalkan",
			eventTaskAssigned: "Tugas dialihkan", eventTaskMoved: "Tugas dipindahkan",
			eventTaskDeleted: "Tugas dihapus", eventDecisionAccepted: "Keputusan diterima",
			eventDecisionRejected: "Keputusan ditolak", eventCommentAdded: "Komentar ditambahkan",
			eventCommentRemoved: "Komentar ditarik",
		},
	},
	"it": {
		many: "{n} aggiornamenti",
		one:  "{n} aggiornamento",
		events: map[string]string{
			eventTaskCreated: "Attività creata", eventTaskStatusChanged: "Attività a {status}",
			eventTaskDone: "Attività completata", eventTaskRejected: "Attività scartata",
			eventTaskAssigned: "Attività riassegnata", eventTaskMoved: "Attività spostata",
			eventTaskDeleted: "Attività eliminata", eventDecisionAccepted: "Decisione accettata",
			eventDecisionRejected: "Decisione respinta", eventCommentAdded: "Commento aggiunto",
			eventCommentRemoved: "Commento ritirato",
		},
	},
	"ja": {
		many: "更新 {n}件",
		events: map[string]string{
			eventTaskCreated: "タスクを作成", eventTaskStatusChanged: "タスクを{status}に変更",
			eventTaskDone: "タスクを完了", eventTaskRejected: "タスクを却下",
			eventTaskAssigned: "タスクの担当を変更", eventTaskMoved: "タスクを移動",
			eventTaskDeleted: "タスクを削除", eventDecisionAccepted: "決定を採択",
			eventDecisionRejected: "決定を却下", eventCommentAdded: "コメントを追加",
			eventCommentRemoved: "コメントを取り消し",
		},
	},
	"ko": {
		many: "업데이트 {n}건",
		events: map[string]string{
			eventTaskCreated: "작업 생성", eventTaskStatusChanged: "작업 {status}",
			eventTaskDone: "작업 완료", eventTaskRejected: "작업 기각",
			eventTaskAssigned: "작업 담당 변경", eventTaskMoved: "작업 이동",
			eventTaskDeleted: "작업 삭제", eventDecisionAccepted: "결정 채택",
			eventDecisionRejected: "결정 기각", eventCommentAdded: "댓글 추가",
			eventCommentRemoved: "댓글 철회",
		},
	},
	"nl": {
		many: "{n} updates",
		one:  "{n} update",
		events: map[string]string{
			eventTaskCreated: "Taak aangemaakt", eventTaskStatusChanged: "Taak op {status}",
			eventTaskDone: "Taak afgerond", eventTaskRejected: "Taak vervallen",
			eventTaskAssigned: "Taak opnieuw toegewezen", eventTaskMoved: "Taak verplaatst",
			eventTaskDeleted: "Taak verwijderd", eventDecisionAccepted: "Besluit aangenomen",
			eventDecisionRejected: "Besluit afgewezen", eventCommentAdded: "Reactie geplaatst",
			eventCommentRemoved: "Reactie ingetrokken",
		},
	},
	"pl": {
		many: "Aktualizacje: {n}",
		one:  "Aktualizacja: {n}",
		events: map[string]string{
			eventTaskCreated: "Utworzono zadanie", eventTaskStatusChanged: "Zadanie: {status}",
			eventTaskDone: "Ukończono zadanie", eventTaskRejected: "Odrzucono zadanie",
			eventTaskAssigned: "Zmieniono przypisanie", eventTaskMoved: "Przeniesiono zadanie",
			eventTaskDeleted: "Usunięto zadanie", eventDecisionAccepted: "Przyjęto decyzję",
			eventDecisionRejected: "Odrzucono decyzję", eventCommentAdded: "Dodano komentarz",
			eventCommentRemoved: "Wycofano komentarz",
		},
	},
	"pt-BR": {
		many: "{n} atualizações",
		one:  "{n} atualização",
		events: map[string]string{
			eventTaskCreated: "Tarefa criada", eventTaskStatusChanged: "Tarefa para {status}",
			eventTaskDone: "Tarefa concluída", eventTaskRejected: "Tarefa recusada",
			eventTaskAssigned: "Tarefa reatribuída", eventTaskMoved: "Tarefa movida",
			eventTaskDeleted: "Tarefa excluída", eventDecisionAccepted: "Decisão aceita",
			eventDecisionRejected: "Decisão rejeitada", eventCommentAdded: "Comentário adicionado",
			eventCommentRemoved: "Comentário retirado",
		},
	},
	"ru": {
		many: "Обновления: {n}",
		one:  "Обновление: {n}",
		events: map[string]string{
			eventTaskCreated: "Создана задача", eventTaskStatusChanged: "Задача: {status}",
			eventTaskDone: "Задача завершена", eventTaskRejected: "Задача отклонена",
			eventTaskAssigned: "Сменён исполнитель", eventTaskMoved: "Задача перенесена",
			eventTaskDeleted: "Задача удалена", eventDecisionAccepted: "Решение принято",
			eventDecisionRejected: "Решение отклонено", eventCommentAdded: "Добавлен комментарий",
			eventCommentRemoved: "Отозван комментарий",
		},
	},
	"th": {
		many: "อัปเดต {n} รายการ",
		events: map[string]string{
			eventTaskCreated: "สร้างงาน", eventTaskStatusChanged: "งานเป็น {status}",
			eventTaskDone: "งานเสร็จแล้ว", eventTaskRejected: "ตีตกงาน",
			eventTaskAssigned: "เปลี่ยนผู้รับผิดชอบ", eventTaskMoved: "ย้ายงาน",
			eventTaskDeleted: "ลบงาน", eventDecisionAccepted: "รับข้อตัดสิน",
			eventDecisionRejected: "ปฏิเสธข้อตัดสิน", eventCommentAdded: "เพิ่มความเห็น",
			eventCommentRemoved: "ถอนความเห็น",
		},
	},
	"tr": {
		many: "{n} güncelleme",
		events: map[string]string{
			eventTaskCreated: "Görev oluşturuldu", eventTaskStatusChanged: "Görev: {status}",
			eventTaskDone: "Görev bitti", eventTaskRejected: "Görevden vazgeçildi",
			eventTaskAssigned: "Görev ataması değişti", eventTaskMoved: "Görev taşındı",
			eventTaskDeleted: "Görev silindi", eventDecisionAccepted: "Karar kabul edildi",
			eventDecisionRejected: "Karar reddedildi", eventCommentAdded: "Yorum eklendi",
			eventCommentRemoved: "Yorum geri alındı",
		},
	},
	"uk": {
		many: "Оновлення: {n}",
		events: map[string]string{
			eventTaskCreated: "Створено завдання", eventTaskStatusChanged: "Завдання: {status}",
			eventTaskDone: "Завдання завершено", eventTaskRejected: "Завдання відхилено",
			eventTaskAssigned: "Змінено виконавця", eventTaskMoved: "Завдання перенесено",
			eventTaskDeleted: "Завдання видалено", eventDecisionAccepted: "Рішення ухвалено",
			eventDecisionRejected: "Рішення відхилено", eventCommentAdded: "Додано коментар",
			eventCommentRemoved: "Відкликано коментар",
		},
	},
	"vi": {
		many: "{n} cập nhật",
		events: map[string]string{
			eventTaskCreated: "Đã tạo công việc", eventTaskStatusChanged: "Công việc: {status}",
			eventTaskDone: "Đã hoàn thành công việc", eventTaskRejected: "Đã bác công việc",
			eventTaskAssigned: "Đã đổi người phụ trách", eventTaskMoved: "Đã chuyển công việc",
			eventTaskDeleted: "Đã xóa công việc", eventDecisionAccepted: "Đã chấp nhận quyết định",
			eventDecisionRejected: "Đã bác quyết định", eventCommentAdded: "Đã thêm bình luận",
			eventCommentRemoved: "Đã thu hồi bình luận",
		},
	},
	"zh-Hans": {
		many: "{n} 项更新",
		events: map[string]string{
			eventTaskCreated: "创建任务", eventTaskStatusChanged: "任务改为{status}",
			eventTaskDone: "完成任务", eventTaskRejected: "否决任务",
			eventTaskAssigned: "变更负责人", eventTaskMoved: "移动任务",
			eventTaskDeleted: "删除任务", eventDecisionAccepted: "采纳决定",
			eventDecisionRejected: "否决决定", eventCommentAdded: "添加评论",
			eventCommentRemoved: "撤回评论",
		},
	},
	"zh-Hant": {
		many: "{n} 項更新",
		events: map[string]string{
			eventTaskCreated: "建立任務", eventTaskStatusChanged: "任務改為{status}",
			eventTaskDone: "完成任務", eventTaskRejected: "否決任務",
			eventTaskAssigned: "變更負責人", eventTaskMoved: "移動任務",
			eventTaskDeleted: "刪除任務", eventDecisionAccepted: "採納決定",
			eventDecisionRejected: "否決決定", eventCommentAdded: "新增評論",
			eventCommentRemoved: "撤回評論",
		},
	},
}

// subjectWordingFor picks the subject vocabulary for a language, resolving the code exactly the
// way the sentences do — one rule for which language a message is in, not two.
func subjectWordingFor(language string) subjectWording {
	if sw, ok := subjectWords[wordingFor(language).language]; ok {
		return sw
	}
	return subjectWords[defaultLanguage]
}

// subjectForOne writes the subject of a message carrying a single event, which is the one case
// where there is room to say what happened rather than how much did.
func subjectForOne(in input, d details) string {
	sw := subjectWordingFor(d.language)
	what, ok := sw.events[in.Event]
	if !ok {
		if what, ok = subjectWords[defaultLanguage].events[in.Event]; !ok {
			what = in.Event
		}
	}
	what = strings.ReplaceAll(what, "{status}", wordingFor(d.language).status(in.New))
	return headerReady(subjectOf(d.project, what+" "+refName(in, d)))
}

// subjectForMany writes the subject of a message carrying a burst. What the events have in common
// by then is the project and how many of them there were, so that is what it says.
//
// It counts one as readily as five. A message carries a single event without this one being able
// to say it in full whenever the run that sends it is not the run the event arrived on — a burst
// ending on an event nobody asked to hear about is exactly that — so the count has to read as well
// at one as it does above it.
func subjectForMany(d details, n int) string {
	sw := subjectWordingFor(d.language)
	count := sw.many
	if n == 1 && sw.one != "" {
		count = sw.one
	}
	if count == "" {
		count = subjectWords[defaultLanguage].many
	}
	return headerReady(subjectOf(d.project, strings.ReplaceAll(count, "{n}", strconv.Itoa(n))))
}

// subjectOf puts the project in front of what happened, and cuts the project — and only the
// project — to fit.
//
// It is the one part of a subject whose length cannot be reckoned with beforehand, so it is the
// one part that gives way. What happened and which record it happened to are never shortened:
// they are what the subject is for, and a subject that runs a few characters long says more than
// one that has been trimmed into saying nothing.
func subjectOf(project, what string) string {
	if strings.TrimSpace(project) == "" {
		project = fallbackProject
	}
	full := "[" + project + "] " + what
	if utf8.RuneCountInString(full) <= subjectLimit {
		return full
	}
	room := subjectLimit - utf8.RuneCountInString("[] "+what) - utf8.RuneCountInString(ellipsis)
	if room < 1 {
		return "[" + ellipsis + "] " + what
	}
	return "[" + string([]rune(project)[:room]) + ellipsis + "] " + what
}

// headerReady makes a subject fit to be a header. Anything outside ASCII travels as an encoded
// word, base64 being the compact way to carry a subject that is mostly not ASCII; a subject that
// is plain ASCII comes back untouched, which is what a reader's mail client would rather see.
func headerReady(subject string) string {
	return mime.BEncoding.Encode("UTF-8", subject)
}
