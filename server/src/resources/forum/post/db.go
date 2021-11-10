package post

import (
	"context"
	"time"

	"github.com/pkg/errors"

	"github.com/openmultiplayer/web/server/src/db"
	"github.com/openmultiplayer/web/server/src/resources/forum"
)

var (
	ErrNoTitle      = errors.New("missing title")
	ErrNoBody       = errors.New("missing body")
	ErrUnauthorised = errors.New("unauthorised")
)

type DB struct {
	db *db.PrismaClient
}

func New(db *db.PrismaClient) Repository {
	return &DB{db}
}

func (d *DB) CreatePost(
	ctx context.Context,
	body, authorID string,
	parentID, replyToID string,
) (*Post, error) {
	short := forum.MakeShortBody(body)

	optional := []db.PostSetParam{
		db.Post.Root.Link(db.Post.ID.Equals(parentID)),
	}

	if replyToID != "" {
		optional = append(optional, db.Post.ReplyTo.Link(db.Post.ID.Equals(replyToID)))
	}

	post, err := d.db.Post.
		CreateOne(
			db.Post.Body.Set(body),
			db.Post.Short.Set(short),
			db.Post.First.Set(false),
			db.Post.Author.Link(db.User.ID.Equals(authorID)),

			optional...,
		).
		With(db.Post.Author.Fetch()).
		Exec(ctx)
	if err != nil {
		return nil, err
	}

	return FromModel(post), nil
}

func (d *DB) GetPosts(ctx context.Context, slug string, max, skip int, deleted, admin bool) ([]Post, error) {
	filters := []db.PostWhereParam{
		db.Post.Or(
			db.Post.And(
				db.Post.First.Equals(true),
				db.Post.Slug.Equals(slug),
			),
			db.Post.And(
				db.Post.First.Equals(false),
				db.Post.Root.Where(db.Post.Slug.Equals(slug)),
			),
		),
	}

	if !admin {
		filters = append(filters, db.Post.Category.Where(
			db.Category.Admin.Equals(false),
		))
	}

	posts, err := d.db.Post.
		FindMany(filters...).
		With(
			db.Post.Author.Fetch(),
			db.Post.Category.Fetch(),
			db.Post.Tags.Fetch(),
			db.Post.ReplyTo.Fetch().With(
				db.Post.Author.Fetch(),
			),
			db.Post.Reacts.Fetch().With(
				db.React.User.Fetch(),
			),
		).
		Take(max).
		Skip(skip).
		OrderBy(db.Post.CreatedAt.Order(db.ASC)).
		Exec(ctx)
	if err != nil {
		return nil, err
	}

	if len(posts) == 0 {
		return nil, nil
	}

	result := []Post{}
	for _, p := range posts {
		// if "show deleted" is false, then filter out posts with a deleted date
		if deleted == false && p.InnerPost.DeletedAt != nil {
			continue
		}

		result = append(result, *FromModel(&p))
	}

	return result, nil
}

func (d *DB) EditPost(ctx context.Context, authorID, id string, title *string, body *string) (*Post, error) {
	// This could probably be optimised. I am too lazy to do it rn.
	post, err := d.db.Post.
		FindUnique(
			db.Post.ID.Equals(id),
		).
		With(db.Post.Author.Fetch()).
		Exec(ctx)
	if err != nil {
		return nil, err
	}
	if post.Author().ID != authorID {
		return nil, ErrUnauthorised
	}

	post, err = d.db.Post.
		FindUnique(
			db.Post.ID.Equals(id),
		).
		With(db.Post.Author.Fetch()).
		Update(
			db.Post.Title.SetIfPresent(title),
			db.Post.Body.SetIfPresent(body),
		).
		Exec(ctx)
	if err != nil {
		return nil, err
	}

	return FromModel(post), err
}

func (d *DB) DeletePost(ctx context.Context, authorID, postID string, force bool) (*Post, error) {
	// This could probably be optimised. I am too lazy to do it rn.
	post, err := d.db.Post.
		FindUnique(
			db.Post.ID.Equals(postID),
		).
		With(
			db.Post.Author.Fetch(),
			db.Post.Tags.Fetch(),
			db.Post.Category.Fetch(),
		).
		Exec(ctx)
	if err != nil {
		return nil, err
	}
	if force == false {
		if post.Author().ID != authorID {
			return nil, ErrUnauthorised
		}
	}

	_, err = d.db.Post.
		FindUnique(db.Post.ID.Equals(postID)).
		Update(
			db.Post.DeletedAt.Set(time.Now()),
		).
		Exec(ctx)

	return FromModel(post), err
}