import matplotlib.pyplot as plt
import sys

def getImageSize(line):
    # find index just after "size="
    index = line.index("size=") + 5
    # read until x
    number = ""
    while (line[index] != 'x'):
        number += line[index]
        index += 1
    # convert to int
    return int(number)

def getThreadCount(line):
    # find index just after "threads="
    index = line.index("threads=") + 8
    # read until '_'
    number = ""
    while (line[index] != '_'):
        number += line[index]
        index += 1
    # convert to int
    return int(number)

def getTimeTaken(line):
    # find index just before " ns/op"
    index = line.index(" ns/op") - 1
    # read numbers in backwards until whitespace
    number = ""
    while (line[index] != ' '):
        number = line[index] + number
        index -= 1
    return int(number)

def plot(imageSize, threadTimeDict):
    threads = threadTimeDict.keys()
    time = threadTimeDict.values()
    plt.bar(threads, time)
    title = "{imageSize}x{imageSize}".format(imageSize=imageSize)
    plt.title(title)
    fname = "{imageSize}x{imageSize}.jpg".format(imageSize=imageSize)
    plt.xlabel("number of threads")
    plt.ylabel("time in seconds")
    plt.savefig(fname, format="jpg")
    
def main():
    fileName = "benchmark.txt"
    f = open(fileName, "r")
    # remove unecessary lines
    lines = f.readlines()
    f.close()
    linesToRemove = []
    for line in lines:
        if (line[0:9] != "Benchmark"):
            linesToRemove.append(line)
    for line in linesToRemove:
        lines.remove(line)
    # for each line:
        # get image-size and thread count
        # make maps for each image-size of thread-count: time taken
        # plot maps
        
        imageSizesMap = dict()
    for line in lines:
        imageSize = getImageSize(line)
        threadCount = getThreadCount(line)
        timeTaken = getTimeTaken(line)
        floatTimeTaken = timeTaken / 1e+9 # nanoseconds to seconds
        # add to dict
        if not imageSizesMap.keys().__contains__(imageSize):
            imageSizesMap[imageSize] = dict()
        imageSizesMap[imageSize][threadCount] = floatTimeTaken
    
    for key, value in imageSizesMap.items():
        plot(key, value)
        print()


if __name__ == "__main__":
    main()