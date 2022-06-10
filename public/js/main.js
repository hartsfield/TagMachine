var tags = (window.location.search.substring(window.location.search.indexOf('=')).substring(1).split(",").filter(v => v));
if (window.location.search.length > 0 && tags.length <= 0) {
    window.location = window.location.origin;
}

function showLogin() {
    if (document.getElementById("loginForm").style.display == "none") {
        document.getElementById("loginForm").style.display = "inline-flex";
    } else {
        document.getElementById("loginForm").style.display = "none";
    }
}

function showPostForm() {
    if (document.getElementById("newPostForm").style.display == "none") {
        document.getElementById("newPostForm").style.display = "block";
    } else {
        document.getElementById("newPostForm").style.display = "none";
    }
}

function showReplyForm(id) {
    if (document.getElementById("replyForm_" + id).style.display == "none") {
        document.getElementById("replyForm_" + id).style.display = "unset";
    } else {
        document.getElementById("replyForm_" + id).style.display = "none";
    }
}

function auth(path) {
    var xhr = new XMLHttpRequest();

    xhr.open('POST', '/api/' + path);
    xhr.setRequestHeader('Content-Type', 'application/json');
    xhr.onload = function() {
        if (xhr.status === 200) {
            console.log(xhr.responseText);
            var res = JSON.parse(xhr.responseText);
            if (res.success == "false") {
                console.log("ERROR", res);
                document.getElementById("errorField").innerHTML = res.error;
            } else {
                window.location.reload();
            }
        }
    };

    xhr.send(JSON.stringify({
                password: document.getElementById("password").value,
                username: document.getElementById("username").value,
    }));
}


function reply(id, parent) {
        console.log("reply");
    var xhr = new XMLHttpRequest();

    xhr.open('POST', '/api/reply');
    xhr.setRequestHeader('Content-Type', 'application/json');
    xhr.onload = function() {
        if (xhr.status === 200) {
            var res = JSON.parse(xhr.responseText);
            if (res.success == "true") {
            } else {
            }
        }
    };

    xhr.send(JSON.stringify({
                body: document.getElementById("replyBody_" + id).value,
                ID: id,
                parent: parent,
    }));
}

function setTag(tag) {
    if (tags.includes(tag)) {
        tags = tags.filter(x => x !== tag);
        window.location = window.location.origin + "/tag/?tags=" + tags.join(",");
        document.getElementById("tag_" + tag).style.backgroundColor = "#d52e12";
    } else {
        tags.push(tag);
        window.location = window.location.origin + "/tag/?tags=" + tags.join(",");
        document.getElementById("tag_" + tag).style.backgroundColor = "#91ba03";
    }
    console.log(tags);
}

function postThread(id) {
    var textarea =  document.getElementById("postBody").value;
    console.log(textarea);
    if (tags.length <= 0 && textarea.length <=5) {
        document.getElementById("postButt").innerHTML = "Please pick at least one #tag from above and include some content";
    } else {
        var xhr = new XMLHttpRequest();

        xhr.open('POST', '/api/newthread');
        xhr.setRequestHeader('Content-Type', 'application/json');
        xhr.onload = function() {
            if (xhr.status === 200) {
                var res = JSON.parse(xhr.responseText);
                if (res.success == "true") {
                    document.getElementById("newPostForm").style.display = "none";
                } else {
                    document.getElementById("postButt").innerHTML = "error";
                }
            }
        };

        xhr.send(JSON.stringify({
                    body: document.getElementById("postBody").value,
                    tags: tags,
        }));
   }
}
