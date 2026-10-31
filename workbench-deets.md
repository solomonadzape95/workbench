# workbench deets
### features
* Image:
1. conversion
2. compression
3. crop
4. bg removal
5. resize
6. enhance? scale up?

* Audio:
1. conversion
2. compression
3. enhance
4. speedup
5. transcibe

* Video:
1. conversion
2. compression
3. extract audio
4. speedup/down
5. transcribe

* Files:
1. conversion
2. compression
3. hash? 
4. edit (docs, pdfs, epubs)

* General:
1. previews
2. batch jobs
3. pipelines (create, save, run)
4. aliases for easy file picking (customizable)
5. keyboard navigation
6. visualization
7. proper tui
8. simplification
9. plugin like capabilities
10. cancellation
11. proper error messaging


### architecture


*SIMPLE SCENARIO: John opens workbench to convert convert 10 pngs to jpgs and remove their backgrounds after, and then convert a pdf into a doc and then edit it.*
###### *PATHWAY: john opens the tui, uses the command palette to search for png to jpg, enters the operation page, and uses aliases to pick out the 10 images, in a combobox type input, and then he chains a remove bg operation to the output of the conversion. he runs it, and moves out to start the pdf conversion, goes through similar steps and starts it. both jobs run in parallel and john can see them both at once, till theyre completed.*


1. ##### how would the actions be carried out? how would they look?: 
actions/operations would be of a certain type, or interface, with some contents like 
`type Operation interface {
    Name() string
    Description() string
    Run(ctx context.Context, input Input, output Output) error
}`
and then all the operations inherit them. so you can just add operations whenever you want. also operations run as ==🔵background tasks== so john can run 2 or more ops, or jobs at the same time and see them in progress. 

For carrying out the ops, we’d need libraries for those that can’t be done offline with normal file manips. would have to research this, to be sure. but yeah. some would be online tools, that might need downloading, maybe models? (transcibing) ==🔵should we have online events? i.e events that call our api to run?== dont know for now. ==🔵i do know i want to create a simple onboarding where the user can select the tools they want to start with, or maybe give them some initial starters. then let them pick what they want in an ops section== 

2. ###### stack? and timeline??
so for this, its probably gonna be my intro to go😅 so… omo. lets see how it goes, im excited, but i still dont exactly have what im gonna do dialed down, and plus its exam time, and ive got that interview, so i have to be super intentional to get this off the ground, ==🔵like an mvp before october’s end?  by God’s grace==. so yeah, ==bubble tea with go==.( i havent actually seen its true power while building tether, but lets see.) then nothing else for now. firstly, i do have to figure out how to get started with go, and go from there 🙂
3. ###### file structure? 
yeah, so while discussing with gpt, it sent soemthing like this…
`workbench/
│
├── cmd/
│   └── workbench/
│       └── main.go
│
├── internal/
│   │
│   ├── core/
│   │   ├── file.go
│   │   ├── operation.go
│   │   ├── job.go
│   │   └── pipeline.go
│   │
│   ├── media/
│   │   ├── image/
│   │   ├── audio/
│   │   └── video/
│   │
│   ├── documents/
│   │   ├── pdf/
│   │   └── epub/
│   │
│   ├── operations/
│   │   ├── convert/
│   │   ├── resize/
│   │   ├── compress/
│   │   ├── crop/
│   │   └── ...
│   │
│   ├── engines/
│   │   ├── ffmpeg/
│   │   ├── imagemagick/
│   │   ├── libreoffice/
│   │   └── ...
│   │
│   └── tui/
│       ├── screens/
│       ├── components/
│       ├── styles/
│       └── app.go
│
├── pipelines/
├── configs/
└── README.md`

…for the file structure, and while i dont wanna comment on that for now, as its still pretty early, i do see some sense in it plus its kinda similar to the one used for tether, and its like go projects are setup with this internal folder strucure. till then.
