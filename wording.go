package main

import (
	"fmt"
	"strings"
	"time"
)

// One event becomes one line, and this is where that line is written.
//
// The language is not this plugin's to ask about. The user told amenbo which language they read
// in, and asking a second time in a setting of this plugin's own would make two answers that can
// disagree — so the code is read back with everything else and the wording is chosen by it. The
// nineteen amenbo itself has are carried here; a code not among them is written in English,
// because a message that says when something happened, to what, and what it was called still does
// its job in the wrong language, and not sending it does not.
//
// The status words are amenbo's own, copied from what it shows on screen rather than translated
// again. A message calling a state one thing while the app calls it another leaves the reader
// looking for something that is not there under that name.

// The status a task moves to, as amenbo names it on the wire.
const (
	statusTodo       = "todo"
	statusInProgress = "in_progress"
	statusDone       = "done"
	statusBlocked    = "blocked"
	statusRejected   = "rejected"
)

// defaultLanguage is what a message is written in when the language amenbo names is one there are
// no sentences for.
const defaultLanguage = "en"

// unknownActor is who a line is attributed to when amenbo names nobody. amenbo displays a name
// for its AI whether or not the user has given it one, so this is for a read that did not come
// back rather than for a user who left it blank.
const unknownActor = "AI"

// timeLayout is how the moment at the head of every line is written. Seconds are part of it
// because a message carries a burst of events, and to the minute a burst is a column of the same
// number saying nothing about the order. The date is on every line, not in a heading, because
// lines that failed to send are carried into the next message and a message can span two days.
const timeLayout = "2006-01-02 15:04:05"

// wording is one language: a sentence for each event, and amenbo's own word for each status.
type wording struct {
	language string
	// sentences hold {who}, {ref} and — for the one event that needs it — {status}.
	sentences map[string]string
	statuses  map[string]string
}

// Most languages say this as a sentence with the actor as its subject. Four do not, and the
// reason is grammatical rather than stylistic: in Korean the particle after a value depends on
// how that value is pronounced, which a reference ending in a digit does not settle, and in
// Polish, Russian and Ukrainian the past tense agrees with the gender of a name this plugin is
// handed and cannot know. Guessing either produces text that reads as broken to the person the
// message is for, so those four are written the way a log is: the actor, then what was done.
var wordings = []wording{
	{
		language: "en",
		statuses: map[string]string{
			statusTodo: "To do", statusInProgress: "In progress", statusDone: "Done",
			statusBlocked: "Blocked", statusRejected: "Rejected",
		},
		sentences: map[string]string{
			eventTaskCreated:       "{who} created {ref}",
			eventTaskStatusChanged: "{who} moved {ref} to {status}",
			eventTaskDone:          "{who} finished {ref}",
			eventTaskRejected:      "{who} decided against {ref}",
			eventTaskAssigned:      "{who} reassigned {ref}",
			eventTaskMoved:         "{who} moved {ref} to another project",
			eventTaskDeleted:       "{who} deleted {ref}",
			eventDecisionAccepted:  "{who} accepted {ref}",
			eventDecisionRejected:  "{who} rejected {ref}",
			eventCommentAdded:      "{who} commented on {ref}",
			eventCommentRemoved:    "{who} took back a comment on {ref}",
		},
	},
	{
		language: "de",
		statuses: map[string]string{
			statusTodo: "Offen", statusInProgress: "In Arbeit", statusDone: "Erledigt",
			statusBlocked: "Blockiert", statusRejected: "Verworfen",
		},
		sentences: map[string]string{
			eventTaskCreated:       "{who} hat {ref} erstellt",
			eventTaskStatusChanged: "{who} hat {ref} auf {status} gesetzt",
			eventTaskDone:          "{who} hat {ref} abgeschlossen",
			eventTaskRejected:      "{who} hat {ref} verworfen",
			eventTaskAssigned:      "{who} hat {ref} neu zugewiesen",
			eventTaskMoved:         "{who} hat {ref} in ein anderes Projekt verschoben",
			eventTaskDeleted:       "{who} hat {ref} gelöscht",
			eventDecisionAccepted:  "{who} hat {ref} angenommen",
			eventDecisionRejected:  "{who} hat {ref} abgelehnt",
			eventCommentAdded:      "{who} hat {ref} kommentiert",
			eventCommentRemoved:    "{who} hat einen Kommentar zu {ref} zurückgenommen",
		},
	},
	{
		language: "es",
		statuses: map[string]string{
			statusTodo: "Pendiente", statusInProgress: "En curso", statusDone: "Hecho",
			statusBlocked: "Bloqueado", statusRejected: "Descartado",
		},
		sentences: map[string]string{
			eventTaskCreated:       "{who} creó {ref}",
			eventTaskStatusChanged: "{who} pasó {ref} a {status}",
			eventTaskDone:          "{who} terminó {ref}",
			eventTaskRejected:      "{who} descartó {ref}",
			eventTaskAssigned:      "{who} reasignó {ref}",
			eventTaskMoved:         "{who} movió {ref} a otro proyecto",
			eventTaskDeleted:       "{who} eliminó {ref}",
			eventDecisionAccepted:  "{who} aceptó {ref}",
			eventDecisionRejected:  "{who} rechazó {ref}",
			eventCommentAdded:      "{who} comentó en {ref}",
			eventCommentRemoved:    "{who} retiró un comentario de {ref}",
		},
	},
	{
		language: "fr",
		statuses: map[string]string{
			statusTodo: "À faire", statusInProgress: "En cours", statusDone: "Terminée",
			statusBlocked: "Bloquée", statusRejected: "Écartée",
		},
		sentences: map[string]string{
			eventTaskCreated:       "{who} a créé {ref}",
			eventTaskStatusChanged: "{who} a fait passer {ref} à {status}",
			eventTaskDone:          "{who} a terminé {ref}",
			eventTaskRejected:      "{who} a écarté {ref}",
			eventTaskAssigned:      "{who} a réattribué {ref}",
			eventTaskMoved:         "{who} a déplacé {ref} vers un autre projet",
			eventTaskDeleted:       "{who} a supprimé {ref}",
			eventDecisionAccepted:  "{who} a accepté {ref}",
			eventDecisionRejected:  "{who} a rejeté {ref}",
			eventCommentAdded:      "{who} a commenté {ref}",
			eventCommentRemoved:    "{who} a retiré un commentaire de {ref}",
		},
	},
	{
		language: "hi",
		statuses: map[string]string{
			statusTodo: "करना है", statusInProgress: "चल रहा है", statusDone: "पूरा",
			statusBlocked: "रुका हुआ", statusRejected: "अस्वीकृत",
		},
		sentences: map[string]string{
			eventTaskCreated:       "{who} ने {ref} बनाया",
			eventTaskStatusChanged: "{who} ने {ref} को {status} किया",
			eventTaskDone:          "{who} ने {ref} पूरा किया",
			eventTaskRejected:      "{who} ने {ref} छोड़ दिया",
			eventTaskAssigned:      "{who} ने {ref} किसी और को सौंपा",
			eventTaskMoved:         "{who} ने {ref} दूसरे प्रोजेक्ट में भेजा",
			eventTaskDeleted:       "{who} ने {ref} मिटाया",
			eventDecisionAccepted:  "{who} ने {ref} स्वीकार किया",
			eventDecisionRejected:  "{who} ने {ref} अस्वीकार किया",
			eventCommentAdded:      "{who} ने {ref} पर टिप्पणी की",
			eventCommentRemoved:    "{who} ने {ref} की टिप्पणी वापस ली",
		},
	},
	{
		language: "id",
		statuses: map[string]string{
			statusTodo: "Akan dikerjakan", statusInProgress: "Sedang dikerjakan", statusDone: "Selesai",
			statusBlocked: "Terhambat", statusRejected: "Ditolak",
		},
		sentences: map[string]string{
			eventTaskCreated:       "{who} membuat {ref}",
			eventTaskStatusChanged: "{who} mengubah {ref} menjadi {status}",
			eventTaskDone:          "{who} menyelesaikan {ref}",
			eventTaskRejected:      "{who} membatalkan {ref}",
			eventTaskAssigned:      "{who} mengalihkan {ref}",
			eventTaskMoved:         "{who} memindahkan {ref} ke proyek lain",
			eventTaskDeleted:       "{who} menghapus {ref}",
			eventDecisionAccepted:  "{who} menerima {ref}",
			eventDecisionRejected:  "{who} menolak {ref}",
			eventCommentAdded:      "{who} mengomentari {ref}",
			eventCommentRemoved:    "{who} menarik komentar pada {ref}",
		},
	},
	{
		language: "it",
		statuses: map[string]string{
			statusTodo: "Da fare", statusInProgress: "In corso", statusDone: "Fatta",
			statusBlocked: "Bloccata", statusRejected: "Scartata",
		},
		sentences: map[string]string{
			eventTaskCreated:       "{who} ha creato {ref}",
			eventTaskStatusChanged: "{who} ha portato {ref} a {status}",
			eventTaskDone:          "{who} ha completato {ref}",
			eventTaskRejected:      "{who} ha scartato {ref}",
			eventTaskAssigned:      "{who} ha riassegnato {ref}",
			eventTaskMoved:         "{who} ha spostato {ref} in un altro progetto",
			eventTaskDeleted:       "{who} ha eliminato {ref}",
			eventDecisionAccepted:  "{who} ha accettato {ref}",
			eventDecisionRejected:  "{who} ha respinto {ref}",
			eventCommentAdded:      "{who} ha commentato {ref}",
			eventCommentRemoved:    "{who} ha ritirato un commento su {ref}",
		},
	},
	{
		language: "ja",
		statuses: map[string]string{
			statusTodo: "未着手", statusInProgress: "進行中", statusDone: "完了",
			statusBlocked: "ブロック", statusRejected: "却下",
		},
		sentences: map[string]string{
			eventTaskCreated:       "{who} が {ref} を作成しました",
			eventTaskStatusChanged: "{who} が {ref} を{status}にしました",
			eventTaskDone:          "{who} が {ref} を完了しました",
			eventTaskRejected:      "{who} が {ref} を却下しました",
			eventTaskAssigned:      "{who} が {ref} の担当を変えました",
			eventTaskMoved:         "{who} が {ref} を別のプロジェクトへ移しました",
			eventTaskDeleted:       "{who} が {ref} を削除しました",
			eventDecisionAccepted:  "{who} が {ref} を採択しました",
			eventDecisionRejected:  "{who} が {ref} を却下しました",
			eventCommentAdded:      "{who} が {ref} にコメントしました",
			eventCommentRemoved:    "{who} が {ref} のコメントを取り消しました",
		},
	},
	{
		language: "ko",
		statuses: map[string]string{
			statusTodo: "할 일", statusInProgress: "진행 중", statusDone: "완료",
			statusBlocked: "막힘", statusRejected: "기각",
		},
		sentences: map[string]string{
			eventTaskCreated:       "{who}: {ref} 생성",
			eventTaskStatusChanged: "{who}: {ref} → {status}",
			eventTaskDone:          "{who}: {ref} 완료",
			eventTaskRejected:      "{who}: {ref} 기각",
			eventTaskAssigned:      "{who}: {ref} 담당 변경",
			eventTaskMoved:         "{who}: {ref} 다른 프로젝트로 이동",
			eventTaskDeleted:       "{who}: {ref} 삭제",
			eventDecisionAccepted:  "{who}: {ref} 채택",
			eventDecisionRejected:  "{who}: {ref} 기각",
			eventCommentAdded:      "{who}: {ref} 댓글 추가",
			eventCommentRemoved:    "{who}: {ref} 댓글 철회",
		},
	},
	{
		language: "nl",
		statuses: map[string]string{
			statusTodo: "Te doen", statusInProgress: "Bezig", statusDone: "Klaar",
			statusBlocked: "Geblokkeerd", statusRejected: "Afgewezen",
		},
		sentences: map[string]string{
			eventTaskCreated:       "{who} heeft {ref} aangemaakt",
			eventTaskStatusChanged: "{who} heeft {ref} op {status} gezet",
			eventTaskDone:          "{who} heeft {ref} afgerond",
			eventTaskRejected:      "{who} heeft {ref} laten vervallen",
			eventTaskAssigned:      "{who} heeft {ref} opnieuw toegewezen",
			eventTaskMoved:         "{who} heeft {ref} naar een ander project verplaatst",
			eventTaskDeleted:       "{who} heeft {ref} verwijderd",
			eventDecisionAccepted:  "{who} heeft {ref} aangenomen",
			eventDecisionRejected:  "{who} heeft {ref} afgewezen",
			eventCommentAdded:      "{who} heeft op {ref} gereageerd",
			eventCommentRemoved:    "{who} heeft een reactie op {ref} ingetrokken",
		},
	},
	{
		language: "pl",
		statuses: map[string]string{
			statusTodo: "Do zrobienia", statusInProgress: "W toku", statusDone: "Gotowe",
			statusBlocked: "Zablokowane", statusRejected: "Odrzucone",
		},
		sentences: map[string]string{
			eventTaskCreated:       "{who}: utworzenie {ref}",
			eventTaskStatusChanged: "{who}: {ref} → {status}",
			eventTaskDone:          "{who}: ukończenie {ref}",
			eventTaskRejected:      "{who}: odrzucenie {ref}",
			eventTaskAssigned:      "{who}: zmiana przypisania {ref}",
			eventTaskMoved:         "{who}: przeniesienie {ref} do innego projektu",
			eventTaskDeleted:       "{who}: usunięcie {ref}",
			eventDecisionAccepted:  "{who}: przyjęcie {ref}",
			eventDecisionRejected:  "{who}: odrzucenie {ref}",
			eventCommentAdded:      "{who}: komentarz do {ref}",
			eventCommentRemoved:    "{who}: wycofanie komentarza do {ref}",
		},
	},
	{
		language: "pt-BR",
		statuses: map[string]string{
			statusTodo: "A fazer", statusInProgress: "Em andamento", statusDone: "Concluída",
			statusBlocked: "Bloqueada", statusRejected: "Recusada",
		},
		sentences: map[string]string{
			eventTaskCreated:       "{who} criou {ref}",
			eventTaskStatusChanged: "{who} mudou {ref} para {status}",
			eventTaskDone:          "{who} concluiu {ref}",
			eventTaskRejected:      "{who} recusou {ref}",
			eventTaskAssigned:      "{who} reatribuiu {ref}",
			eventTaskMoved:         "{who} moveu {ref} para outro projeto",
			eventTaskDeleted:       "{who} excluiu {ref}",
			eventDecisionAccepted:  "{who} aceitou {ref}",
			eventDecisionRejected:  "{who} rejeitou {ref}",
			eventCommentAdded:      "{who} comentou em {ref}",
			eventCommentRemoved:    "{who} retirou um comentário de {ref}",
		},
	},
	{
		language: "ru",
		statuses: map[string]string{
			statusTodo: "К выполнению", statusInProgress: "В работе", statusDone: "Готово",
			statusBlocked: "Заблокировано", statusRejected: "Отклонено",
		},
		sentences: map[string]string{
			eventTaskCreated:       "{who}: создание {ref}",
			eventTaskStatusChanged: "{who}: {ref} → {status}",
			eventTaskDone:          "{who}: завершение {ref}",
			eventTaskRejected:      "{who}: отклонение {ref}",
			eventTaskAssigned:      "{who}: смена исполнителя {ref}",
			eventTaskMoved:         "{who}: перенос {ref} в другой проект",
			eventTaskDeleted:       "{who}: удаление {ref}",
			eventDecisionAccepted:  "{who}: принятие {ref}",
			eventDecisionRejected:  "{who}: отклонение {ref}",
			eventCommentAdded:      "{who}: комментарий к {ref}",
			eventCommentRemoved:    "{who}: отзыв комментария к {ref}",
		},
	},
	{
		language: "th",
		statuses: map[string]string{
			statusTodo: "รอทำ", statusInProgress: "กำลังทำ", statusDone: "เสร็จแล้ว",
			statusBlocked: "ติดขัด", statusRejected: "ตีตก",
		},
		sentences: map[string]string{
			eventTaskCreated:       "{who} สร้าง {ref}",
			eventTaskStatusChanged: "{who} เปลี่ยน {ref} เป็น {status}",
			eventTaskDone:          "{who} ทำ {ref} เสร็จแล้ว",
			eventTaskRejected:      "{who} ตีตก {ref}",
			eventTaskAssigned:      "{who} เปลี่ยนผู้รับผิดชอบ {ref}",
			eventTaskMoved:         "{who} ย้าย {ref} ไปโปรเจกต์อื่น",
			eventTaskDeleted:       "{who} ลบ {ref}",
			eventDecisionAccepted:  "{who} รับ {ref}",
			eventDecisionRejected:  "{who} ปฏิเสธ {ref}",
			eventCommentAdded:      "{who} แสดงความเห็นใน {ref}",
			eventCommentRemoved:    "{who} ถอนความเห็นใน {ref}",
		},
	},
	{
		language: "tr",
		statuses: map[string]string{
			statusTodo: "Yapılacak", statusInProgress: "Sürüyor", statusDone: "Bitti",
			statusBlocked: "Engelli", statusRejected: "Reddedildi",
		},
		sentences: map[string]string{
			eventTaskCreated:       "{who} {ref} kaydını oluşturdu",
			eventTaskStatusChanged: "{who} {ref} kaydını {status} yaptı",
			eventTaskDone:          "{who} {ref} kaydını bitirdi",
			eventTaskRejected:      "{who} {ref} kaydından vazgeçti",
			eventTaskAssigned:      "{who} {ref} atamasını değiştirdi",
			eventTaskMoved:         "{who} {ref} kaydını başka projeye taşıdı",
			eventTaskDeleted:       "{who} {ref} kaydını sildi",
			eventDecisionAccepted:  "{who} {ref} kararını kabul etti",
			eventDecisionRejected:  "{who} {ref} kararını reddetti",
			eventCommentAdded:      "{who} {ref} kaydına yorum yaptı",
			eventCommentRemoved:    "{who} {ref} kaydındaki yorumu geri aldı",
		},
	},
	{
		language: "uk",
		statuses: map[string]string{
			statusTodo: "До виконання", statusInProgress: "У роботі", statusDone: "Готово",
			statusBlocked: "Заблоковано", statusRejected: "Відхилено",
		},
		sentences: map[string]string{
			eventTaskCreated:       "{who}: створення {ref}",
			eventTaskStatusChanged: "{who}: {ref} → {status}",
			eventTaskDone:          "{who}: завершення {ref}",
			eventTaskRejected:      "{who}: відхилення {ref}",
			eventTaskAssigned:      "{who}: зміна виконавця {ref}",
			eventTaskMoved:         "{who}: перенесення {ref} до іншого проєкту",
			eventTaskDeleted:       "{who}: видалення {ref}",
			eventDecisionAccepted:  "{who}: ухвалення {ref}",
			eventDecisionRejected:  "{who}: відхилення {ref}",
			eventCommentAdded:      "{who}: коментар до {ref}",
			eventCommentRemoved:    "{who}: відкликання коментаря до {ref}",
		},
	},
	{
		language: "vi",
		statuses: map[string]string{
			statusTodo: "Cần làm", statusInProgress: "Đang làm", statusDone: "Xong",
			statusBlocked: "Bị chặn", statusRejected: "Bị bác",
		},
		sentences: map[string]string{
			eventTaskCreated:       "{who} đã tạo {ref}",
			eventTaskStatusChanged: "{who} đã chuyển {ref} sang {status}",
			eventTaskDone:          "{who} đã hoàn thành {ref}",
			eventTaskRejected:      "{who} đã bác {ref}",
			eventTaskAssigned:      "{who} đã đổi người phụ trách {ref}",
			eventTaskMoved:         "{who} đã chuyển {ref} sang dự án khác",
			eventTaskDeleted:       "{who} đã xóa {ref}",
			eventDecisionAccepted:  "{who} đã chấp nhận {ref}",
			eventDecisionRejected:  "{who} đã bác bỏ {ref}",
			eventCommentAdded:      "{who} đã bình luận về {ref}",
			eventCommentRemoved:    "{who} đã thu hồi bình luận về {ref}",
		},
	},
	{
		language: "zh-Hans",
		statuses: map[string]string{
			statusTodo: "待办", statusInProgress: "进行中", statusDone: "已完成",
			statusBlocked: "受阻", statusRejected: "已否决",
		},
		sentences: map[string]string{
			eventTaskCreated:       "{who} 创建了 {ref}",
			eventTaskStatusChanged: "{who} 把 {ref} 改为{status}",
			eventTaskDone:          "{who} 完成了 {ref}",
			eventTaskRejected:      "{who} 否决了 {ref}",
			eventTaskAssigned:      "{who} 变更了 {ref} 的负责人",
			eventTaskMoved:         "{who} 把 {ref} 移到了其他项目",
			eventTaskDeleted:       "{who} 删除了 {ref}",
			eventDecisionAccepted:  "{who} 采纳了 {ref}",
			eventDecisionRejected:  "{who} 否决了 {ref}",
			eventCommentAdded:      "{who} 评论了 {ref}",
			eventCommentRemoved:    "{who} 撤回了 {ref} 的评论",
		},
	},
	{
		language: "zh-Hant",
		statuses: map[string]string{
			statusTodo: "待辦", statusInProgress: "進行中", statusDone: "已完成",
			statusBlocked: "受阻", statusRejected: "已否決",
		},
		sentences: map[string]string{
			eventTaskCreated:       "{who} 建立了 {ref}",
			eventTaskStatusChanged: "{who} 把 {ref} 改為{status}",
			eventTaskDone:          "{who} 完成了 {ref}",
			eventTaskRejected:      "{who} 否決了 {ref}",
			eventTaskAssigned:      "{who} 變更了 {ref} 的負責人",
			eventTaskMoved:         "{who} 把 {ref} 移到了其他專案",
			eventTaskDeleted:       "{who} 刪除了 {ref}",
			eventDecisionAccepted:  "{who} 採納了 {ref}",
			eventDecisionRejected:  "{who} 否決了 {ref}",
			eventCommentAdded:      "{who} 評論了 {ref}",
			eventCommentRemoved:    "{who} 撤回了 {ref} 的評論",
		},
	},
}

// wordingFor picks the language a message is written in.
//
// An exact code is answered exactly. A code carrying a region or a script this has no sentences
// for — "ja-JP", or "zh" on its own — falls back to the first language sharing its primary
// subtag, which is why the list above is a slice: the answer to "zh" has to be the same one every
// time. Anything left over is English.
func wordingFor(language string) wording {
	language = strings.TrimSpace(language)
	for _, w := range wordings {
		if strings.EqualFold(w.language, language) {
			return w
		}
	}
	if primary, _, _ := strings.Cut(language, "-"); primary != "" {
		for _, w := range wordings {
			if base, _, _ := strings.Cut(w.language, "-"); strings.EqualFold(base, primary) {
				return w
			}
		}
	}
	return english()
}

// english is what everything falls back to. It is looked up rather than taken by position, so
// nothing depends on where in the list it sits.
func english() wording {
	for _, w := range wordings {
		if w.language == defaultLanguage {
			return w
		}
	}
	return wording{language: defaultLanguage}
}

// sentence is how this language says one event happened. A language missing one falls back to
// English rather than to nothing, and an event no language has a sentence for — which would have
// to be one amenbo added after this build — is written under its own name, so it is still
// reported rather than silently dropped.
func (w wording) sentence(event string) string {
	if s, ok := w.sentences[event]; ok {
		return s
	}
	if s, ok := english().sentences[event]; ok {
		return s
	}
	return "{who}: " + event + " {ref}"
}

// status is amenbo's own word for a state. One it has no word for is written as amenbo wrote it,
// which is the same rule: a state added later still reads.
func (w wording) status(status string) string {
	if s, ok := w.statuses[status]; ok {
		return s
	}
	return status
}

// eventLine writes one event as the one line it becomes in a message:
//
//	2026-08-01 14:32:05  Sora created AMB-T-<n> — Ship the thing
//
// The moment leads, so a column of them can be read down. The title trails behind a dash, and a
// title that could not be read back simply is not there — what happened, and to what, is already
// on the line without it.
func eventLine(in input, d details) string {
	w := wordingFor(d.language)
	sentence := strings.NewReplacer(
		"{who}", actorName(d),
		"{ref}", refName(in, d),
		"{status}", w.status(in.New),
	).Replace(w.sentence(in.Event))

	line := localTime(in.At) + "  " + sentence
	if d.title != "" {
		line += " — " + d.title
	}
	return line
}

// actorName is who the line is about.
func actorName(d details) string {
	if d.aiName == "" {
		return unknownActor
	}
	return d.aiName
}

// refName is what the line is about. amenbo's own rendering is used where it came back; where it
// did not, the number out of the payload stands in — which on a comment names the comment rather
// than the task it was written on, that being all this run managed to learn.
func refName(in input, d details) string {
	if d.ref == "" {
		return fmt.Sprintf("#%d", in.ID)
	}
	return d.ref
}

// localTime writes the moment an event happened in the time of the machine reading it. The
// payload carries UTC, and the person this is for is at the machine this runs on, so there is no
// reason to hand them a conversion to do. A moment that will not parse is written as it arrived:
// unreadable in the way it was given, rather than replaced by a time that is not the truth.
func localTime(at string) string {
	t, err := time.Parse(time.RFC3339, at)
	if err != nil {
		return at
	}
	return t.Local().Format(timeLayout)
}
