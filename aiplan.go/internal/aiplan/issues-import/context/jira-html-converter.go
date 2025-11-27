// Пакет context предоставляет функции для обработки и замены ссылок и других элементов в HTML-коде, полученном из различных источников (например, Jira, комментарии).  Он предназначен для улучшения читаемости и удобства использования HTML, а также для обеспечения безопасности при отображении внешних ссылок.
//
// Основные возможности:
//   - Замена ссылок на комментарии Jira на ссылки на сами комментарии.
//   - Замена ссылок на проблемы Jira на внутренние ссылки в текущей рабочей области.
//   - Форматирование кода.
//   - Замена ссылок на изображения на ссылки на изображения в хранилище.
//   - Удаление нежелательных элементов (например, иконок).
//   - Обработка ссылок на внешние ресурсы с проверкой домена.
//   - Поддержка различных форматов изображений (GIF, JPEG, PNG).
package context

import (
	"bytes"
	"fmt"
	"image"
	"log/slog"
	"net/url"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/dao"
	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/issues-import/entity"
	"github.com/aisa-it/aiplan/aiplan.go/internal/aiplan/issues-import/utils"
	"github.com/andygrunwald/go-jira"
	"github.com/gofrs/uuid"
	"golang.org/x/net/html"

	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
)

func (c *ImportContext) replaceAttachments(src string, issue *dao.Issue, comment *dao.IssueComment) (string, error) {
	doc, err := html.Parse(strings.NewReader(src))
	if err != nil {
		return "", err
	}

	var f func(*html.Node)
	f = func(n *html.Node) {
		// Replace font color
		if formatFontColor(n) {
			return
		}

		// Format pre code
		if formatCode(n) {
			return
		}

		// Remove icon
		if n.Type == html.ElementNode && n.Data == "img" {
			for _, a := range n.Attr {
				if a.Key == "class" && a.Val == "rendericon" {
					n.Parent.RemoveChild(n)
					return
				}
			}
		}

		// Replace mentions
		if n.Type == html.ElementNode && n.Data == "a" && getAttr(n, "class") == "user-hover" {
			rel := getAttr(n, "rel")
			if rel == "" {
				return
			}

			u, err := c.Users.Get(rel)
			if err != nil {
				c.Log.Error("Get user", "rel", rel, "err", err)
				return
			}

			name := u.Email
			if u.Username != nil {
				name = *u.Username
			}

			htmlRemoveChildren(n)
			n.AppendChild(&html.Node{
				Type: html.TextNode,
				Data: fmt.Sprintf("@%s", name),
			})

			n.Data = "span"
			n.Attr = []html.Attribute{
				{Key: "class", Val: "mention"},
				{Key: "data-type", Val: "mention"},
				{Key: "data-id", Val: name},
				{Key: "data-label", Val: u.Email},
			}

			return
		}

		// Replace comment links to internal
		if c.replaceCommentLinks(n) {
			return
		}

		// Replace links to internal
		if c.replaceIssueLinks(n) {
			return
		}

		// Replace attachment
		if n.Type == html.ElementNode && n.Data == "a" || n.Data == "img" {
			for i, a := range n.Attr {
				if a.Key == "href" || a.Key == "src" {
					arr := strings.Split(a.Val, "/")
					id := a.Val
					if len(arr) > 2 {
						id = arr[len(arr)-2]
					}
					attachment, _ := c.Attachments.Get(id)
					if attachment == nil || attachment.IssueAttachment == nil {
						break
					}
					n.Attr[i].Val = "/uploads/" + attachment.IssueAttachment.AssetId.String()
					break
				}
			}
		}

		// Replace images
		if !c.IgnoreAttachments {
			ok, attach := c.formatImg(n)
			if ok {
				if issue != nil {
					attach.InlineAsset.IssueId = uuid.NullUUID{Valid: true, UUID: issue.ID}
					attach.InlineAsset.WorkspaceId = &issue.WorkspaceId
				} else if comment != nil {
					attach.InlineAsset.CommentId = uuid.NullUUID{UUID: comment.Id, Valid: true}
					attach.InlineAsset.WorkspaceId = &comment.WorkspaceId
				} else {
					slog.Warn("Empty issue and comment for img formatting", "attachmentId", attach.JiraAttachment.ID)
					return
				}

				if attach.JiraAttachment != nil {
					metadataUrl := fmt.Sprintf("rest/api/2/attachment/%s", attach.JiraAttachment.ID)
					req, _ := c.Client.NewRequest("GET", metadataUrl, nil)
					if _, err := c.Client.Do(req, attach.JiraAttachment); err != nil {
						slog.Error("Get inline attachment metadata", "url", metadataUrl, "err", err)
						return
					}

					attach.InlineAsset.FileSize = attach.JiraAttachment.Size

					if a, _ := c.Attachments.Get(attach.JiraAttachment.ID); a == nil {
						c.Attachments.Put(attach.JiraAttachment.ID, attach)
					}
				} else if attach.FullURL != nil {
					if a, _ := c.Attachments.Get(attach.FullURL.String()); a == nil {
						c.Attachments.Put(attach.FullURL.String(), attach)
					}
				}
				return
			}
		}

		for c := n.FirstChild; c != nil; c = c.NextSibling {
			f(c)
		}
	}
	f(doc)

	var buf bytes.Buffer
	if err := html.Render(&buf, doc); err != nil {
		return "", err
	}

	h := strings.ReplaceAll(buf.String(), "\t", "")
	h = strings.ReplaceAll(h, "\n", "")
	h = strings.ReplaceAll(strings.ReplaceAll(h, "<html><head></head><body>", ""), "</body></html>", "")
	return h, nil
}

func formatFontColor(node *html.Node) bool {
	if node.Type != html.ElementNode || node.Data != "font" {
		return false
	}

	node.Data = "span"
	color := ""
	for _, attr := range node.Attr {
		if attr.Key == "color" {
			color = attr.Val
		}
	}
	node.Attr = []html.Attribute{{Key: "style", Val: fmt.Sprintf("color: %s", color)}}
	return true
}

func (context *ImportContext) formatImg(node *html.Node) (bool, *entity.Attachment) {
	if node.Type != html.ElementNode || node.Data != "span" {
		return false, nil
	}
	classFound := false
	for _, attr := range node.Attr {
		if attr.Key == "class" && attr.Val == "image-wrap" {
			classFound = true
			break
		}
	}
	if !classFound {
		return false, nil
	}

	attachmentId := ""
	fileName := ""
	imgWidth := 0
	var fullURL *url.URL

	eachChild(node, func(c *html.Node) bool {
		if c.Data == "a" {
			for _, attr := range c.Attr {
				switch attr.Key {
				case "title":
					fileName = attr.Val
				case "file-preview-id":
					attachmentId = attr.Val
				}
			}

			eachChild(c, func(img *html.Node) bool {
				if img.Data == "img" {
					src := ""
					for _, attr := range img.Attr {
						switch attr.Key {
						case "src":
							src = attr.Val
						case "width":
							imgWidth, _ = strconv.Atoi(attr.Val)
						}
					}

					// Fetch width from thumb if not specified
					if imgWidth == 0 {
						imgWidth = context.getImgWidth(src)
					}

					return true
				}
				return false
			})

			return true
		} else if c.Data == "img" && attachmentId == "" {
			// If thumbnail not provided
			src := ""
			for _, attr := range c.Attr {
				switch attr.Key {
				case "src":
					src = attr.Val
				case "width":
					imgWidth, _ = strconv.Atoi(attr.Val)
				}
			}

			// Get attachmentID from img URL
			imgURL, err := url.Parse(src)
			if err != nil {
				slog.Error("Parse img url", "src", src, "err", err)
				return true
			}
			arr := strings.Split(imgURL.Path, "/")

			// Fetch width from thumb if not specified
			if imgWidth == 0 {
				imgWidth = context.getImgWidth(src)
			}

			if strings.Contains(imgURL.Path, "attachment") {
				attachmentId = arr[len(arr)-2]
			} else {
				fullURL = imgURL
			}
		}
		return false
	})

	var attachment *entity.Attachment
	if attachmentId != "" {
		attachment, _ = context.Attachments.Get(attachmentId) // Search by attachmentId
	} else if fullURL != nil {
		attachment, _ = context.Attachments.Get(fullURL.String()) // Search by attachment URL
	} else {
		return false, nil
	}

	asset := dao.FileAsset{}
	if attachment == nil {
		asset = dao.FileAsset{
			Id:        dao.GenUUID(),
			CreatedAt: time.Now(),
			Name:      fileName,
		}
	} else {
		asset.Id = attachment.DstAssetID
	}

	link := fmt.Sprintf("/api/file/%s", asset.Id.String())

	htmlRemoveChildren(node)
	node.Data = "img"
	node.Attr = []html.Attribute{{Key: "src", Val: link}}

	if imgWidth > 0 {
		node.Attr = append(node.Attr, html.Attribute{Key: "style", Val: fmt.Sprintf("width: %d", imgWidth)})
	}

	newAttach := &entity.Attachment{InlineAsset: &asset, DstAssetID: asset.Id}

	if attachmentId != "" {
		newAttach.JiraAttachment = &jira.Attachment{ID: attachmentId}
	} else {
		newAttach.FullURL = fullURL
	}

	return true, newAttach
}

func eachChild(node *html.Node, fn func(c *html.Node) bool) {
	for c := node.FirstChild; c != nil; c = c.NextSibling {
		if fn(c) {
			return
		}
	}
}

func (c *ImportContext) getImgWidth(src string) int {
	req, _ := c.Client.NewRequest("GET", src, nil)
	resp, err := c.Client.Do(req, nil)
	if err != nil {
		slog.Error("Fetch thumb image", "url", err)
		return 0
	}
	defer resp.Body.Close()

	cfg, _, err := image.DecodeConfig(resp.Body)
	if err != nil {
		slog.Error("Decode thumb image", "url", src, "err", err)
	}
	return cfg.Width
}

func formatCode(node *html.Node) bool {
	if node.Type != html.ElementNode || node.Data != "div" {
		return false
	}
	idFound := false
	for _, attr := range node.Attr {
		if attr.Key == "id" && attr.Val == "syntaxplugin" {
			idFound = true
			break
		}
	}
	if !idFound {
		return false
	}

	code := getText(node)
	htmlRemoveChildren(node)
	node.Data = "pre"
	node.Attr = nil
	node.AppendChild(&html.Node{
		Type: html.TextNode,
		Data: code,
	})
	return true
}

func htmlRemoveChildren(node *html.Node) {
	children := []*html.Node{}
	for c := node.FirstChild; c != nil; c = c.NextSibling {
		if c.FirstChild != nil {
			htmlRemoveChildren(c)
		}
		children = append(children, c)
	}
	for _, child := range children {
		node.RemoveChild(child)
	}
}

func getText(node *html.Node) string {
	res := ""
	for c := node.FirstChild; c != nil; c = c.NextSibling {
		if c.Type == html.TextNode {
			s := strings.TrimSpace(c.Data)
			if len(s) > 0 {
				res += s + "\n"
			}
		} else {
			res += getText(c)
		}
	}
	return res
}

func getAttr(node *html.Node, key string) string {
	for _, attr := range node.Attr {
		if attr.Key == key {
			return attr.Val
		}
	}
	return ""
}

// <a href="https://tc.aisa.ru/browse/IIT-485?focusedCommentId=608877&amp;page=com.atlassian.jira.plugin.system.issuetabpanels%3Acomment-tabpanel#comment-608877" class="external-link" rel="nofollow">комментария</a>
func (c *ImportContext) replaceCommentLinks(n *html.Node) bool {
	if !(n.Type == html.ElementNode && n.Data == "a" && getAttr(n, "class") == "external-link") {
		return false
	}

	commUrl, err := url.Parse(getAttr(n, "href"))
	if err != nil {
		c.Log.Error("Parse issue comment link", "url", getAttr(n, "href"), "err", err)
		return false
	}

	commId := commUrl.Query().Get("focusedCommentId")
	if commId == "" {
		return false
	}
	comment := c.IssueComments.Get(commId)

	origCommentText := strings.TrimPrefix(strings.TrimSpace(getText(n)), "#")

	htmlRemoveChildren(n)

	if !comment.Id.IsNil() {
		// Comment from this import
		n.Attr = []html.Attribute{
			{Key: "data-type", Val: "issue"},
			{Key: "data-slug", Val: comment.WorkspaceId},
			{Key: "data-project-identifier", Val: comment.ProjectId},
			{Key: "data-current-issue-id", Val: comment.IssueId},
			{Key: "data-comment-id", Val: comment.Id.String()},
			{Key: "class", Val: "special-link-mention"},
			{Key: "contenteditable", Val: "false"},
		}
		n.AppendChild(&html.Node{
			Type: html.TextNode,
			Data: "#" + origCommentText,
		})
	} else {
		// Comment from another project
		projectKey, issueKey := utils.ParseRawKey(path.Base(commUrl.Path))

		n.Attr = []html.Attribute{
			{Key: "data-type", Val: "issue"},
			{Key: "data-slug", Val: c.TargetWorkspaceID},
			{Key: "data-project-identifier", Val: projectKey},
			{Key: "data-current-issue-id", Val: issueKey},
			{Key: "data-comment-id", Val: commId},
			{Key: "class", Val: "special-link-mention"},
			{Key: "contenteditable", Val: "false"},
			{Key: "data-original-url", Val: commUrl.String()},
		}
		n.AppendChild(&html.Node{
			Type: html.TextNode,
			Data: "#" + origCommentText,
		})
	}
	n.Data = "span"

	return true
}

func (c *ImportContext) replaceIssueLinks(n *html.Node) bool {
	if !(n.Type == html.ElementNode && n.Data == "a" && (getAttr(n, "class") == "issue-link" || getAttr(n, "class") == "external-link")) {
		return false
	}

	u, err := url.Parse(getAttr(n, "href"))
	if err != nil {
		return false
	}

	// Check host in link
	if u.Host != c.Client.GetBaseURL().Host {
		return false
	}

	arr := strings.Split(strings.TrimSuffix(strings.TrimPrefix(u.Path, "/"), "/"), "/")

	// Check if path contains browse element
	if len(arr) < 2 || arr[0] != "browse" {
		return false
	}

	issueKey := arr[1]
	project, seq := utils.ParseKey(jira.Issue{Key: issueKey})

	// Issue from another project
	if project != c.ProjectKey {
		return false
	}

	newLink := fmt.Sprintf("/%s/projects/%s/issues/%s", c.TargetWorkspaceID, c.Project.ID, seq)

	n.Attr = []html.Attribute{
		{Key: "href", Val: newLink},
		{Key: "target", Val: "_blank"},
		{Key: "rel", Val: "noopener noreferrer nofollow"},
	}

	htmlRemoveChildren(n)

	n.AppendChild(&html.Node{
		Type: html.TextNode,
		Data: issueKey,
	})

	return true
}
